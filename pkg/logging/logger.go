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
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// SeverityInfo indicates an informational log entry.
	SeverityInfo = "INFO"

	// SeverityWarn indicates a warning log entry.
	SeverityWarn = "WARN"

	// SeverityError indicates an error log entry.
	SeverityError = "ERROR"

	// logDirPermissions is the permission mode for the log directory.
	logDirPermissions = 0700

	// logFilePermissions is the permission mode for individual log files.
	logFilePermissions = 0600
)

// Logger writes structured log entries to date-based files in a directory.
// Each log entry is appended to a file named <YYYYMMDD>.logs.txt. The logger
// opens and closes the file for each write to minimize memory usage and
// avoid keeping file handles open during high connection volumes.
type Logger struct {
	dir string
	mu  sync.Mutex
}

// NewLogger creates a new Logger that writes to the given directory.
// The directory is created if it does not exist.
func NewLogger(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, logDirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create log directory %q: %w", dir, err)
	}

	return &Logger{dir: dir}, nil
}

// Log writes a single log entry with the given tag, severity, and message.
// The entry is appended to the log file for the current date.
func (l *Logger) Log(tag, severity, message string) error {
	now := time.Now()

	dateStr := now.Format("20060102")
	timeStr := now.Format("20060102 15:04:05.000")

	filename := filepath.Join(l.dir, dateStr+".logs.txt")
	line := fmt.Sprintf("[%s] [%s] [%s]\t%s\n", timeStr, tag, severity, message)

	l.mu.Lock()
	defer l.mu.Unlock()

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePermissions)
	if err != nil {
		return fmt.Errorf("failed to open log file %q: %w", filename, err)
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// Warn writes a warning-level log entry.
func (l *Logger) Warn(tag, message string) error {
	return l.Log(tag, SeverityWarn, message)
}

// Info writes an info-level log entry.
func (l *Logger) Info(tag, message string) error {
	return l.Log(tag, SeverityInfo, message)
}

// Error writes an error-level log entry.
func (l *Logger) Error(tag, message string) error {
	return l.Log(tag, SeverityError, message)
}
