// Copyright © 2026 Marcel Joachim Kloubert <marcel@kloubert.dev>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package tunnel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackoffDelay_ExponentialGrowth(t *testing.T) {
	cfg := ReconnectConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Factor:       2.0,
		Jitter:       0.0, // no jitter for deterministic results
	}

	expected := []time.Duration{
		1 * time.Second,  // attempt 0: 1s * 2^0 = 1s
		2 * time.Second,  // attempt 1: 1s * 2^1 = 2s
		4 * time.Second,  // attempt 2: 1s * 2^2 = 4s
		8 * time.Second,  // attempt 3: 1s * 2^3 = 8s
		16 * time.Second, // attempt 4: 1s * 2^4 = 16s
		30 * time.Second, // attempt 5: 1s * 2^5 = 32s, capped at 30s
	}

	for i, want := range expected {
		got := cfg.delay(i)
		if got != want {
			t.Errorf("attempt %d: got %v, want %v", i, got, want)
		}
	}
}

func TestBackoffDelay_WithJitter(t *testing.T) {
	cfg := ReconnectConfig{
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Factor:       2.0,
		Jitter:       0.25,
	}

	const samples = 100

	for attempt := 0; attempt < 3; attempt++ {
		baseDelay := cfg.InitialDelay
		for i := 0; i < attempt; i++ {
			baseDelay = time.Duration(float64(baseDelay) * cfg.Factor)
		}
		if baseDelay > cfg.MaxDelay {
			baseDelay = cfg.MaxDelay
		}

		minAllowed := time.Duration(float64(baseDelay) * (1.0 - cfg.Jitter))
		maxAllowed := time.Duration(float64(baseDelay) * (1.0 + cfg.Jitter))

		for i := 0; i < samples; i++ {
			got := cfg.delay(attempt)
			if got < minAllowed || got > maxAllowed {
				t.Errorf("attempt %d, sample %d: delay %v outside range [%v, %v]",
					attempt, i, got, minAllowed, maxAllowed)
			}
		}
	}
}

func TestRunWithReconnect_SucceedsFirstTry(t *testing.T) {
	var connectCalls atomic.Int32

	// Use a mutex-protected channel that gets created fresh each connect call.
	var mu sync.Mutex
	var currentDone chan struct{}

	cfg := ReconnectConfig{
		MaxRetries:   0, // disabled: succeed once, no reconnect
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Factor:       2.0,
		Jitter:       0.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connect := func() error {
		connectCalls.Add(1)
		ch := make(chan struct{})
		mu.Lock()
		currentDone = ch
		mu.Unlock()
		// Simulate a connection that ends after a short delay.
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(ch)
		}()
		return nil
	}

	done := func() <-chan struct{} {
		mu.Lock()
		defer mu.Unlock()
		return currentDone
	}

	cleanup := func() {}

	err := RunWithReconnect(ctx, cfg, nil, connect, done, cleanup)

	// With MaxRetries=0, connect succeeds once, done fires, and function returns.
	calls := connectCalls.Load()
	if calls != 1 {
		t.Errorf("expected exactly 1 connect call, got %d", calls)
	}

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestRunWithReconnect_RetriesOnFailure(t *testing.T) {
	var connectCalls atomic.Int32
	errConnect := errors.New("connection failed")

	cfg := ReconnectConfig{
		MaxRetries:   5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Factor:       2.0,
		Jitter:       0.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	var currentDone chan struct{}

	connect := func() error {
		n := connectCalls.Add(1)
		if n <= 2 {
			return errConnect
		}
		// Third call succeeds. Create a done channel that closes shortly,
		// and cancel the context so the test ends cleanly.
		ch := make(chan struct{})
		mu.Lock()
		currentDone = ch
		mu.Unlock()
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		return nil
	}

	done := func() <-chan struct{} {
		mu.Lock()
		defer mu.Unlock()
		return currentDone
	}

	var cleanupCalls atomic.Int32
	cleanup := func() {
		cleanupCalls.Add(1)
	}

	err := RunWithReconnect(ctx, cfg, nil, connect, done, cleanup)

	// Connect was called exactly 3 times: 2 failures + 1 success.
	calls := connectCalls.Load()
	if calls != 3 {
		t.Errorf("expected 3 connect calls (2 failures + 1 success), got %d", calls)
	}

	// Cleanup should have been called at least for the failed attempts.
	if cleanupCalls.Load() < 2 {
		t.Errorf("expected at least 2 cleanup calls, got %d", cleanupCalls.Load())
	}

	// Should return context.Canceled since we cancelled context to end the test.
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunWithReconnect_ExhaustsRetries(t *testing.T) {
	var connectCalls atomic.Int32
	errConnect := errors.New("always fails")

	cfg := ReconnectConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Factor:       2.0,
		Jitter:       0.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connect := func() error {
		connectCalls.Add(1)
		return errConnect
	}

	done := func() <-chan struct{} {
		return make(chan struct{}) // never fires
	}

	cleanup := func() {}

	err := RunWithReconnect(ctx, cfg, nil, connect, done, cleanup)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// First call + 2 retries = 3 total calls.
	calls := connectCalls.Load()
	if calls != 3 {
		t.Errorf("expected 3 connect calls (1 initial + 2 retries), got %d", calls)
	}
}

func TestRunWithReconnect_CancelledContext(t *testing.T) {
	var connectCalls atomic.Int32

	cfg := ReconnectConfig{
		MaxRetries:   -1, // infinite retries
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Factor:       2.0,
		Jitter:       0.0,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 250ms.
	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()

	errConnect := errors.New("connection failed")

	connect := func() error {
		connectCalls.Add(1)
		return errConnect
	}

	done := func() <-chan struct{} {
		return make(chan struct{}) // never fires
	}

	var cleanupCalls atomic.Int32
	cleanup := func() {
		cleanupCalls.Add(1)
	}

	err := RunWithReconnect(ctx, cfg, nil, connect, done, cleanup)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	if connectCalls.Load() < 1 {
		t.Error("expected at least 1 connect call")
	}
}

func TestRunWithReconnect_DisabledWithZeroRetries(t *testing.T) {
	var connectCalls atomic.Int32
	doneCh := make(chan struct{})

	cfg := ReconnectConfig{
		MaxRetries:   0, // disabled
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Factor:       2.0,
		Jitter:       0.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connect := func() error {
		connectCalls.Add(1)
		// Simulate a connection that ends after a short delay.
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(doneCh)
		}()
		return nil
	}

	done := func() <-chan struct{} {
		return doneCh
	}

	cleanup := func() {}

	err := RunWithReconnect(ctx, cfg, nil, connect, done, cleanup)

	// With MaxRetries=0, connect succeeds once, then done fires and
	// function should return without reconnecting.
	calls := connectCalls.Load()
	if calls != 1 {
		t.Errorf("expected exactly 1 connect call, got %d", calls)
	}

	// Should return nil since the connection succeeded and then done fired.
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}
