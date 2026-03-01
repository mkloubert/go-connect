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

package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewLogger_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")

	_, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("log directory does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("log path is not a directory")
	}
}

func TestLogger_Log_CreatesDateFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if err := logger.Log("TEST", "INFO", "hello world"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	expectedFile := filepath.Join(dir, time.Now().Format("20060102")+".logs.txt")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Fatalf("expected log file does not exist: %v", err)
	}
}

func TestLogger_Log_Format(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if err := logger.Log("AUTH", "WARN", "test message"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	expectedFile := filepath.Join(dir, time.Now().Format("20060102")+".logs.txt")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	line := strings.TrimSpace(string(data))

	// Format: [YYYYMMDD hh:mm:ss.zzz] [TAG] [SEVERITY]\tMESSAGE
	pattern := `^\[\d{8} \d{2}:\d{2}:\d{2}\.\d{3}\] \[AUTH\] \[WARN\]\ttest message$`
	matched, err := regexp.MatchString(pattern, line)
	if err != nil {
		t.Fatalf("regexp error: %v", err)
	}
	if !matched {
		t.Fatalf("log line does not match expected format:\ngot:  %q\nwant: %s", line, pattern)
	}
}

func TestLogger_Log_AppendsToFile(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if err := logger.Log("TAG1", "INFO", "first"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	if err := logger.Log("TAG2", "WARN", "second"); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	expectedFile := filepath.Join(dir, time.Now().Format("20060102")+".logs.txt")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(data))
	}

	if !strings.Contains(lines[0], "[TAG1]") || !strings.Contains(lines[0], "first") {
		t.Errorf("first line unexpected: %q", lines[0])
	}
	if !strings.Contains(lines[1], "[TAG2]") || !strings.Contains(lines[1], "second") {
		t.Errorf("second line unexpected: %q", lines[1])
	}
}

func TestLogger_Log_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	const numGoroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = logger.Warn("CONCURRENT", "message from goroutine")
		}(i)
	}

	wg.Wait()

	expectedFile := filepath.Join(dir, time.Now().Format("20060102")+".logs.txt")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != numGoroutines {
		t.Fatalf("expected %d lines, got %d", numGoroutines, len(lines))
	}
}

func TestLogger_SeverityHelpers(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if err := logger.Info("T", "info msg"); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if err := logger.Warn("T", "warn msg"); err != nil {
		t.Fatalf("Warn() error = %v", err)
	}
	if err := logger.Error("T", "error msg"); err != nil {
		t.Fatalf("Error() error = %v", err)
	}

	expectedFile := filepath.Join(dir, time.Now().Format("20060102")+".logs.txt")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[INFO]") {
		t.Error("missing INFO entry")
	}
	if !strings.Contains(content, "[WARN]") {
		t.Error("missing WARN entry")
	}
	if !strings.Contains(content, "[ERROR]") {
		t.Error("missing ERROR entry")
	}
}
