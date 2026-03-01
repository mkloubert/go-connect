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

package geoblock

import (
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// testDBPath returns the absolute path to the test MaxMind database.
func testDBPath(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to determine test file path via runtime.Caller")
	}

	return filepath.Join(filepath.Dir(filename), "testdata", "GeoIP2-City-Test.mmdb")
}

func TestOpen_Success(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB", "US"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	countries := db.Countries()
	if len(countries) != 2 {
		t.Fatalf("expected 2 countries, got %d", len(countries))
	}
}

func TestOpen_FileNotFound(t *testing.T) {
	_, err := Open("/nonexistent/path/to/database.mmdb", []string{"GB"})
	if err == nil {
		t.Fatal("Open should return an error for a missing file")
	}
}

func TestOpen_EmptyCountries(t *testing.T) {
	db, err := Open(testDBPath(t), []string{})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	countries := db.Countries()
	if len(countries) != 0 {
		t.Fatalf("expected 0 countries, got %d", len(countries))
	}

	// With an empty block list, nothing should be blocked.
	blocked, code := db.IsBlocked("81.2.69.142")
	if blocked {
		t.Errorf("expected not blocked with empty country list, got blocked with code %q", code)
	}
}

func TestIsBlocked_BlockedCountry(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("81.2.69.142")
	if !blocked {
		t.Error("expected 81.2.69.142 (GB) to be blocked when GB is in the list")
	}
	if code != "GB" {
		t.Errorf("expected country code GB, got %q", code)
	}
}

func TestIsBlocked_AllowedCountry(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"US"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("81.2.69.142")
	if blocked {
		t.Error("expected 81.2.69.142 (GB) NOT to be blocked when only US is in the list")
	}
	if code != "GB" {
		t.Errorf("expected country code GB, got %q", code)
	}
}

func TestIsBlocked_CaseInsensitive(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"gb"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("81.2.69.142")
	if !blocked {
		t.Error("expected 81.2.69.142 (GB) to be blocked when 'gb' (lowercase) is in the list")
	}
	if code != "GB" {
		t.Errorf("expected country code GB, got %q", code)
	}
}

func TestIsBlocked_TrimmedCountryCodes(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"  GB  "})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("81.2.69.142")
	if !blocked {
		t.Error("expected 81.2.69.142 (GB) to be blocked when '  GB  ' (with spaces) is in the list")
	}
	if code != "GB" {
		t.Errorf("expected country code GB, got %q", code)
	}
}

func TestIsBlocked_InvalidIP(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("not-an-ip")
	if blocked {
		t.Error("expected invalid IP to not be blocked")
	}
	if code != "" {
		t.Errorf("expected empty country code for invalid IP, got %q", code)
	}
}

func TestIsBlocked_PrivateIP(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("192.168.1.1")
	if blocked {
		t.Error("expected private IP 192.168.1.1 to not be blocked (no geo data)")
	}
	if code != "" {
		t.Errorf("expected empty country code for private IP, got %q", code)
	}
}

func TestIsBlocked_IPv6(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"KR"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("2001:220::")
	if !blocked {
		t.Error("expected 2001:220:: (KR) to be blocked when KR is in the list")
	}
	if code != "KR" {
		t.Errorf("expected country code KR, got %q", code)
	}
}

func TestIsBlocked_IPv6Allowed(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"US"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("2001:220::")
	if blocked {
		t.Error("expected 2001:220:: (KR) NOT to be blocked when only US is in the list")
	}
	if code != "KR" {
		t.Errorf("expected country code KR, got %q", code)
	}
}

func TestIsBlocked_WhitespaceIP(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("  81.2.69.142  ")
	if !blocked {
		t.Error("expected '  81.2.69.142  ' (with whitespace) to be blocked")
	}
	if code != "GB" {
		t.Errorf("expected country code GB, got %q", code)
	}
}

func TestCountries(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"  gb  ", "us", " KR "})
	if err != nil {
		t.Fatalf("Open returned unexpected error: %v", err)
	}
	defer db.Close()

	countries := db.Countries()
	sort.Strings(countries)

	expected := []string{"GB", "KR", "US"}
	if len(countries) != len(expected) {
		t.Fatalf("expected %d countries, got %d: %v", len(expected), len(countries), countries)
	}
	for i, c := range countries {
		if c != expected[i] {
			t.Errorf("expected country[%d] = %q, got %q", i, expected[i], c)
		}
	}
}
