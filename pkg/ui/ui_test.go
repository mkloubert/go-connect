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

package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestSuccess_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Success("operation completed")
	out := buf.String()

	if !strings.Contains(out, "✓") {
		t.Errorf("expected output to contain '✓', got: %q", out)
	}
	if !strings.Contains(out, "operation completed") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
	if strings.Contains(out, "\033[") {
		t.Errorf("expected no ANSI codes in no-color mode, got: %q", out)
	}
}

func TestSuccess_WithColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, true, false, false)

	u.Success("ok")
	out := buf.String()

	if !strings.Contains(out, "\033[32m") {
		t.Errorf("expected green ANSI code '\\033[32m', got: %q", out)
	}
	if !strings.Contains(out, "\033[0m") {
		t.Errorf("expected reset ANSI code '\\033[0m', got: %q", out)
	}
}

func TestError_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Error("something failed")
	out := buf.String()

	if !strings.Contains(out, "✗") {
		t.Errorf("expected output to contain '✗', got: %q", out)
	}
	if !strings.Contains(out, "something failed") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
}

func TestWarning_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Warning("be careful")
	out := buf.String()

	if !strings.Contains(out, "⚠") {
		t.Errorf("expected output to contain '⚠', got: %q", out)
	}
	if !strings.Contains(out, "be careful") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
}

func TestHint_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Hint("try this instead")
	out := buf.String()

	if !strings.Contains(out, "Hint:") {
		t.Errorf("expected output to contain 'Hint:', got: %q", out)
	}
	if !strings.Contains(out, "try this instead") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
}

func TestDebug_VerboseEnabled(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, true, false)

	u.Debug("internal state")
	out := buf.String()

	if !strings.Contains(out, "[DEBUG]") {
		t.Errorf("expected output to contain '[DEBUG]', got: %q", out)
	}
	if !strings.Contains(out, "internal state") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
}

func TestDebug_VerboseDisabled(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Debug("should not appear")
	out := buf.String()

	if out != "" {
		t.Errorf("expected empty output when verbose is disabled, got: %q", out)
	}
}

func TestQuietMode_SuccessSuppressed(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, true)

	u.Success("should not appear")
	out := buf.String()

	if out != "" {
		t.Errorf("expected empty output in quiet mode for Success, got: %q", out)
	}
}

func TestQuietMode_ErrorNotSuppressed(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, true)

	u.Error("critical failure")
	out := buf.String()

	if !strings.Contains(out, "✗") {
		t.Errorf("expected Error to still show in quiet mode, got: %q", out)
	}
	if !strings.Contains(out, "critical failure") {
		t.Errorf("expected error message in quiet mode, got: %q", out)
	}
}

func TestQuietMode_RawNotSuppressed(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, true)

	u.Raw("raw output")
	out := buf.String()

	if !strings.Contains(out, "raw output") {
		t.Errorf("expected Raw to still show in quiet mode, got: %q", out)
	}
}

func TestInfo_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Info("some information")
	out := buf.String()

	if !strings.Contains(out, "some information") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
	if strings.Contains(out, "\033[") {
		t.Errorf("expected no ANSI codes for Info in no-color mode, got: %q", out)
	}
}

func TestBullet_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Bullet("list item")
	out := buf.String()

	if !strings.Contains(out, "•") {
		t.Errorf("expected output to contain '•', got: %q", out)
	}
	if !strings.Contains(out, "list item") {
		t.Errorf("expected output to contain message, got: %q", out)
	}
}

func TestHeader_NoColor(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.Header("Section Title")
	out := buf.String()

	if !strings.Contains(out, "Section Title") {
		t.Errorf("expected output to contain header text, got: %q", out)
	}
}

func TestBlankLine(t *testing.T) {
	var buf bytes.Buffer
	u := NewUI(&buf, false, false, false)

	u.BlankLine()
	out := buf.String()

	if out != "\n" {
		t.Errorf("expected output to be exactly '\\n', got: %q", out)
	}
}
