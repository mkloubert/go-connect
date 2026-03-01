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

package ipsum

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testFeed = `# IPsum Threat Intelligence Feed
# (https://github.com/stamparm/ipsum)
#
# Last update: Sun, 01 Mar 2026 03:00:31 +0100
#
# IP	number of (black)lists
#
1.2.3.4	10
5.6.7.8	3
10.0.0.1	2
192.168.1.1	1
203.0.113.50	5
`

func TestDB_LoadFromReader(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	// Only IPs with count >= 3 should be stored.
	if got := db.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
}

func TestDB_IsBlocked_HighCount(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("1.2.3.4")
	if !blocked {
		t.Error("expected 1.2.3.4 to be blocked")
	}
	if count != 10 {
		t.Errorf("count = %d, want 10", count)
	}
}

func TestDB_IsBlocked_AtThreshold(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("5.6.7.8")
	if !blocked {
		t.Error("expected 5.6.7.8 to be blocked (count=3, threshold=3)")
	}
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}

func TestDB_IsBlocked_BelowThreshold(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("10.0.0.1")
	if blocked {
		t.Error("expected 10.0.0.1 to NOT be blocked (count=2, threshold=3)")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 (below threshold, not stored)", count)
	}
}

func TestDB_IsBlocked_NotInList(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("8.8.8.8")
	if blocked {
		t.Error("expected 8.8.8.8 to NOT be blocked")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestDB_IsBlocked_InvalidIP(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("not-an-ip")
	if blocked {
		t.Error("expected invalid IP to NOT be blocked")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestDB_IsBlocked_IPv6(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("::1")
	if blocked {
		t.Error("expected IPv6 to NOT be blocked (IPsum only covers IPv4)")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestDB_IsBlocked_WhitespaceIP(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(testFeed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}

	blocked, count := db.IsBlocked("  1.2.3.4  ")
	if !blocked {
		t.Error("expected trimmed IP to be blocked")
	}
	if count != 10 {
		t.Errorf("count = %d, want 10", count)
	}
}

func TestDB_LoadFromReader_ReplacesPreviousData(t *testing.T) {
	db := NewDB(3)

	feed1 := "1.2.3.4\t10\n"
	if err := db.LoadFromReader(strings.NewReader(feed1), nil); err != nil {
		t.Fatalf("first load error = %v", err)
	}
	if db.Len() != 1 {
		t.Fatalf("after first load: Len() = %d, want 1", db.Len())
	}

	feed2 := "5.6.7.8\t5\n"
	if err := db.LoadFromReader(strings.NewReader(feed2), nil); err != nil {
		t.Fatalf("second load error = %v", err)
	}
	if db.Len() != 1 {
		t.Fatalf("after second load: Len() = %d, want 1", db.Len())
	}

	blocked, _ := db.IsBlocked("1.2.3.4")
	if blocked {
		t.Error("1.2.3.4 should no longer be blocked after reload")
	}

	blocked, _ = db.IsBlocked("5.6.7.8")
	if !blocked {
		t.Error("5.6.7.8 should be blocked after reload")
	}
}

func TestDB_LoadFromReader_WarnsOnMalformedLines(t *testing.T) {
	feed := `# comment
1.2.3.4	10
not-an-ip	5
5.6.7.8	abc
9.10.11.12
`
	db := NewDB(3)

	var warnings []int
	warn := func(lineNum int, line string) {
		warnings = append(warnings, lineNum)
	}

	if err := db.LoadFromReader(strings.NewReader(feed), warn); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}
	if db.Len() != 1 {
		t.Fatalf("Len() = %d, want 1 (only 1.2.3.4 is valid with count>=3)", db.Len())
	}

	// Lines 3 (not-an-ip), 4 (abc count), 5 (missing count) should warn.
	if len(warnings) != 3 {
		t.Fatalf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}
	if warnings[0] != 3 || warnings[1] != 4 || warnings[2] != 5 {
		t.Errorf("warning line numbers = %v, want [3, 4, 5]", warnings)
	}
}

func TestDB_LoadFromReader_NoWarnOnNilCallback(t *testing.T) {
	feed := "not-an-ip\t5\n"
	db := NewDB(3)

	// Should not panic with nil warn func.
	if err := db.LoadFromReader(strings.NewReader(feed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}
}

func TestDB_LoadFromReader_EmptyFeed(t *testing.T) {
	db := NewDB(3)
	if err := db.LoadFromReader(strings.NewReader(""), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}
	if db.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", db.Len())
	}
}

func TestNewDB_MinCountFloor(t *testing.T) {
	db := NewDB(0)
	feed := "1.2.3.4\t1\n"
	if err := db.LoadFromReader(strings.NewReader(feed), nil); err != nil {
		t.Fatalf("LoadFromReader() error = %v", err)
	}
	blocked, _ := db.IsBlocked("1.2.3.4")
	if !blocked {
		t.Error("minCount=0 should be floored to 1, and count=1 should match")
	}
}

func TestDB_LoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ipsum.txt")

	if err := os.WriteFile(path, []byte(testFeed), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	db := NewDB(3)
	if err := db.LoadFromFile(path, nil); err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if got := db.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}

	blocked, count := db.IsBlocked("1.2.3.4")
	if !blocked || count != 10 {
		t.Errorf("1.2.3.4: blocked=%v, count=%d; want blocked=true, count=10", blocked, count)
	}
}

func TestDB_LoadFromFile_NotFound(t *testing.T) {
	db := NewDB(3)
	err := db.LoadFromFile("/nonexistent/path/ipsum.txt", nil)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestDB_LoadFromFile_WarnsOnMalformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ipsum.txt")

	feed := "# comment\n1.2.3.4\t10\nbadline\n"
	if err := os.WriteFile(path, []byte(feed), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	db := NewDB(3)

	var warnings []int
	warn := func(lineNum int, line string) {
		warnings = append(warnings, lineNum)
	}

	if err := db.LoadFromFile(path, warn); err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0] != 3 {
		t.Errorf("warning line = %d, want 3", warnings[0])
	}
}
