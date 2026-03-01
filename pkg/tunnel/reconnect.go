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
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// ReconnectCallback is called on reconnect events to provide UI feedback.
type ReconnectCallback func(event ReconnectEvent)

// ReconnectEvent describes a reconnect attempt for UI feedback.
type ReconnectEvent struct {
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	Err        error
	Connected  bool
}

// ReconnectConfig configures the reconnect behavior.
type ReconnectConfig struct {
	MaxRetries   int           // -1 = infinite, 0 = disabled
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Factor       float64
	Jitter       float64 // 0.0 to 1.0 (fraction of delay)
}

// DefaultReconnectConfig returns a default reconnect configuration with
// sensible defaults: 10 retries, 1s initial delay, 30s max delay,
// factor of 2.0, and 25% jitter.
func DefaultReconnectConfig() ReconnectConfig {
	return ReconnectConfig{
		MaxRetries:   10,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Factor:       2.0,
		Jitter:       0.25,
	}
}

// delay computes the exponential backoff delay for the given attempt
// number: InitialDelay * Factor^attempt, capped at MaxDelay, with
// random jitter applied as a fraction of the computed delay.
func (c ReconnectConfig) delay(attempt int) time.Duration {
	d := float64(c.InitialDelay) * math.Pow(c.Factor, float64(attempt))

	if d > float64(c.MaxDelay) {
		d = float64(c.MaxDelay)
	}

	if c.Jitter > 0 {
		// Generate a cryptographically random float in [-1, 1).
		jitterFraction := cryptoRandFloat64()
		// Scale to [-Jitter, +Jitter] of the base delay.
		d = d * (1.0 + c.Jitter*jitterFraction)
	}

	result := time.Duration(d)
	if result < 0 {
		result = 0
	}

	return result
}

// cryptoRandFloat64 returns a random float64 in the range [-1.0, 1.0)
// using crypto/rand for security.
func cryptoRandFloat64() float64 {
	var buf [8]byte
	_, err := rand.Read(buf[:])
	if err != nil {
		// Fallback: return 0 if crypto/rand fails (should not happen).
		return 0
	}

	// Use the first 8 bytes as a uint64 and convert to [0.0, 1.0).
	n := binary.BigEndian.Uint64(buf[:])
	f := float64(n) / float64(math.MaxUint64)

	// Map from [0.0, 1.0) to [-1.0, 1.0).
	return 2.0*f - 1.0
}

// RunWithReconnect runs a connect function with automatic reconnection
// using exponential backoff.
//
// Behavior:
//  1. Call connect(). If it succeeds, wait for done() channel or ctx.Done().
//  2. If connect() fails and MaxRetries==0, return the error immediately.
//  3. If connect() fails or done() fires, enter the reconnect loop.
//  4. In the reconnect loop: call callback with attempt info, wait for the
//     delay, then retry connect.
//  5. On success: call callback with Connected=true, wait for done() again,
//     and reset the attempt counter.
//  6. If max retries are exhausted, return an error wrapping the last error.
//  7. If ctx is cancelled at any point, call cleanup and return ctx.Err().
//  8. cleanup() is called between reconnect attempts (after disconnect,
//     before retry) to release resources.
func RunWithReconnect(
	ctx context.Context,
	cfg ReconnectConfig,
	callback ReconnectCallback,
	connect func() error,
	done func() <-chan struct{},
	cleanup func(),
) error {
	// Initial connection attempt.
	err := connect()
	if err != nil {
		if cfg.MaxRetries == 0 {
			return err
		}
		// Fall through to reconnect loop with the error.
	} else {
		// Connection succeeded. Wait for done or context cancellation.
		select {
		case <-ctx.Done():
			cleanup()
			return ctx.Err()
		case <-done():
			// Connection lost. If MaxRetries==0, just return.
			if cfg.MaxRetries == 0 {
				return nil
			}
			// Fall through to reconnect loop.
			err = fmt.Errorf("connection lost")
		}
	}

	// Reconnect loop.
	var lastErr error
	if err != nil {
		lastErr = err
	}

	for attempt := 0; ; attempt++ {
		// Check if retries are exhausted.
		if cfg.MaxRetries >= 0 && attempt >= cfg.MaxRetries {
			return fmt.Errorf("reconnect: max retries (%d) exhausted: %w", cfg.MaxRetries, lastErr)
		}

		// Check for context cancellation.
		select {
		case <-ctx.Done():
			cleanup()
			return ctx.Err()
		default:
		}

		// Call cleanup before retry to release resources from the
		// previous connection attempt.
		cleanup()

		d := cfg.delay(attempt)

		// Notify callback about the reconnect attempt.
		if callback != nil {
			callback(ReconnectEvent{
				Attempt:    attempt + 1,
				MaxRetries: cfg.MaxRetries,
				Delay:      d,
				Err:        lastErr,
				Connected:  false,
			})
		}

		// Wait for the backoff delay or context cancellation.
		select {
		case <-ctx.Done():
			cleanup()
			return ctx.Err()
		case <-time.After(d):
		}

		// Retry connect.
		err = connect()
		if err != nil {
			lastErr = err
			continue
		}

		// Connection succeeded. Notify callback.
		if callback != nil {
			callback(ReconnectEvent{
				Attempt:    attempt + 1,
				MaxRetries: cfg.MaxRetries,
				Connected:  true,
			})
		}

		// Reset attempt counter on successful reconnect.
		attempt = -1 // will be incremented to 0 at loop top

		// Wait for done or context cancellation.
		select {
		case <-ctx.Done():
			cleanup()
			return ctx.Err()
		case <-done():
			lastErr = fmt.Errorf("connection lost")
			// Continue reconnect loop.
		}
	}
}
