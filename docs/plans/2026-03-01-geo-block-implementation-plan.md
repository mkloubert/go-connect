# Geo-Block Feature Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add optional country-based IP blocking to the broker using MaxMind's GeoLite2-City.mmdb database, blocking connections from specified countries before the handshake.

**Architecture:** New `pkg/geoblock/` package wrapping `github.com/oschwald/geoip2-golang/v2`. Follows the existing `pkg/ipsum/` pattern: standalone package with `DB` struct, integrated into the broker via `ServerOption`, checked in `acceptLoop()` before the handshake. Supports both IPv4 and IPv6.

**Tech Stack:** `github.com/oschwald/geoip2-golang/v2` (v2.1.0), MaxMind test fixtures from `github.com/maxmind/MaxMind-DB` for unit tests.

---

## Phase 1: Core `geoblock` Package

### Task 1: Download test fixture and add dependency

**Files:**
- Create: `pkg/geoblock/testdata/GeoIP2-City-Test.mmdb`
- Modify: `go.mod`, `go.sum`

**Step 1: Add the geoip2-golang dependency**

Run:
```bash
cd /workspace && go get github.com/oschwald/geoip2-golang/v2@latest
```

Expected: `go.mod` updated with `github.com/oschwald/geoip2-golang/v2` and its transitive dependency `github.com/oschwald/maxminddb-golang/v2`.

**Step 2: Download the MaxMind test database**

Run:
```bash
mkdir -p /workspace/pkg/geoblock/testdata
curl -sL "https://github.com/maxmind/MaxMind-DB/raw/main/test-data/GeoIP2-City-Test.mmdb" \
  -o /workspace/pkg/geoblock/testdata/GeoIP2-City-Test.mmdb
```

Expected: Binary file created. Verify with:
```bash
file /workspace/pkg/geoblock/testdata/GeoIP2-City-Test.mmdb
```
Should show: data file or similar (not empty/HTML).

**Step 3: Run `go mod tidy`**

Run:
```bash
cd /workspace && go mod tidy
```

Expected: Clean exit, no errors.

---

### Task 2: Write failing tests for `pkg/geoblock/`

**Files:**
- Create: `pkg/geoblock/geoblock_test.go`

**Step 1: Write the failing tests**

Create `pkg/geoblock/geoblock_test.go` with the copyright header and these tests:

```go
// [COPYRIGHT HEADER]

package geoblock

import (
	"path/filepath"
	"runtime"
	"testing"
)

// testDBPath returns the absolute path to the test mmdb fixture.
func testDBPath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to get test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata", "GeoIP2-City-Test.mmdb")
}

func TestOpen_Success(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB", "us"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
}

func TestOpen_FileNotFound(t *testing.T) {
	_, err := Open("/nonexistent/GeoLite2-City.mmdb", []string{"US"})
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestOpen_EmptyCountries(t *testing.T) {
	db, err := Open(testDBPath(t), []string{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// With no blocked countries, nothing should be blocked.
	blocked, _ := db.IsBlocked("81.2.69.142")
	if blocked {
		t.Error("expected no blocking with empty country list")
	}
}

func TestIsBlocked_BlockedCountry(t *testing.T) {
	// 81.2.69.142 is in GB per MaxMind test data.
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("81.2.69.142")
	if !blocked {
		t.Error("expected 81.2.69.142 (GB) to be blocked")
	}
	if code != "GB" {
		t.Errorf("country code = %q, want %q", code, "GB")
	}
}

func TestIsBlocked_AllowedCountry(t *testing.T) {
	// 81.2.69.142 is in GB, but we only block US.
	db, err := Open(testDBPath(t), []string{"US"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, _ := db.IsBlocked("81.2.69.142")
	if blocked {
		t.Error("expected 81.2.69.142 (GB) to NOT be blocked when only US is blocked")
	}
}

func TestIsBlocked_CaseInsensitive(t *testing.T) {
	// Country codes should be case-insensitive: "gb" matches "GB".
	db, err := Open(testDBPath(t), []string{"gb"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, _ := db.IsBlocked("81.2.69.142")
	if !blocked {
		t.Error("expected case-insensitive match: 'gb' should block GB IP")
	}
}

func TestIsBlocked_TrimmedCountryCodes(t *testing.T) {
	// Country codes should be trimmed: " GB " matches "GB".
	db, err := Open(testDBPath(t), []string{"  GB  "})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, _ := db.IsBlocked("81.2.69.142")
	if !blocked {
		t.Error("expected trimmed match: ' GB ' should block GB IP")
	}
}

func TestIsBlocked_InvalidIP(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("not-an-ip")
	if blocked {
		t.Error("expected invalid IP to NOT be blocked")
	}
	if code != "" {
		t.Errorf("country code = %q, want empty", code)
	}
}

func TestIsBlocked_PrivateIP(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Private IPs have no geo data; should not be blocked.
	blocked, code := db.IsBlocked("192.168.1.1")
	if blocked {
		t.Error("expected private IP to NOT be blocked")
	}
	if code != "" {
		t.Errorf("country code = %q, want empty", code)
	}
}

func TestIsBlocked_IPv6(t *testing.T) {
	// 2001:220:: is in KR per MaxMind test data.
	db, err := Open(testDBPath(t), []string{"KR"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, code := db.IsBlocked("2001:220::")
	if !blocked {
		t.Error("expected 2001:220:: (KR) to be blocked")
	}
	if code != "KR" {
		t.Errorf("country code = %q, want %q", code, "KR")
	}
}

func TestIsBlocked_IPv6Allowed(t *testing.T) {
	// 2001:220:: is in KR, but we only block US.
	db, err := Open(testDBPath(t), []string{"US"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, _ := db.IsBlocked("2001:220::")
	if blocked {
		t.Error("expected 2001:220:: (KR) to NOT be blocked when only US is blocked")
	}
}

func TestIsBlocked_WhitespaceIP(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"GB"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	blocked, _ := db.IsBlocked("  81.2.69.142  ")
	if !blocked {
		t.Error("expected trimmed IP to be blocked")
	}
}

func TestCountries(t *testing.T) {
	db, err := Open(testDBPath(t), []string{"gb", " US ", "kR"})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	countries := db.Countries()
	if len(countries) != 3 {
		t.Fatalf("Countries() len = %d, want 3", len(countries))
	}

	expected := map[string]bool{"GB": true, "US": true, "KR": true}
	for _, c := range countries {
		if !expected[c] {
			t.Errorf("unexpected country %q in Countries()", c)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /workspace && go test ./pkg/geoblock/ -v -count=1
```

Expected: Compilation error — package `geoblock` does not exist yet.

---

### Task 3: Implement `pkg/geoblock/geoblock.go`

**Files:**
- Create: `pkg/geoblock/geoblock.go`

**Step 1: Write the implementation**

Create `pkg/geoblock/geoblock.go`:

```go
// [COPYRIGHT HEADER]

package geoblock

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/oschwald/geoip2-golang/v2"
)

const (
	// DefaultDBPath is the default file name for the GeoLite2-City database.
	// The file must exist in the current working directory.
	DefaultDBPath = "GeoLite2-City.mmdb"
)

// DB holds an open GeoLite2-City database and a set of blocked country codes.
// It provides fast IP-to-country lookups and blocking decisions.
// The DB is safe for concurrent use from multiple goroutines.
type DB struct {
	reader    *geoip2.Reader
	blocked   map[string]struct{}
}

// Open opens a GeoLite2-City (or GeoIP2-City) mmdb file and prepares
// the blocked country set. Country codes are normalized: trimmed of
// whitespace and converted to uppercase.
func Open(dbPath string, countries []string) (*DB, error) {
	reader, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("geoblock: failed to open %q: %w", dbPath, err)
	}

	blocked := make(map[string]struct{}, len(countries))
	for _, c := range countries {
		code := strings.ToUpper(strings.TrimSpace(c))
		if code != "" {
			blocked[code] = struct{}{}
		}
	}

	return &DB{
		reader:  reader,
		blocked: blocked,
	}, nil
}

// IsBlocked reports whether the given IP address belongs to a blocked
// country. It returns the blocked status and the ISO country code.
// For IPs that cannot be parsed, have no geo data, or are not in a
// blocked country, blocked is false and countryCode is empty.
func (db *DB) IsBlocked(ipStr string) (blocked bool, countryCode string) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil {
		return false, ""
	}

	record, err := db.reader.City(addr)
	if err != nil {
		return false, ""
	}

	if !record.HasData() {
		return false, ""
	}

	code := record.Country.ISOCode
	if code == "" {
		return false, ""
	}

	_, isBlocked := db.blocked[strings.ToUpper(code)]
	if isBlocked {
		return true, code
	}

	return false, code
}

// Countries returns the list of blocked country codes (uppercase).
func (db *DB) Countries() []string {
	result := make([]string, 0, len(db.blocked))
	for c := range db.blocked {
		result = append(result, c)
	}
	return result
}

// Close closes the underlying mmdb reader.
func (db *DB) Close() error {
	return db.reader.Close()
}
```

**Step 2: Run tests to verify they pass**

Run:
```bash
cd /workspace && go test ./pkg/geoblock/ -v -count=1
```

Expected: All tests PASS.

**Step 3: Run all existing tests to check for regressions**

Run:
```bash
cd /workspace && go test ./... -timeout 60s
```

Expected: All tests PASS.

---

## Phase 2: Broker Integration

### Task 4: Add `WithGeoFilter` ServerOption and acceptLoop check

**Files:**
- Modify: `pkg/broker/server.go`

**Step 1: Add the import, field, option, log tag, and log method**

In `pkg/broker/server.go`:

1. Add import: `"github.com/mkloubert/go-connect/pkg/geoblock"`
2. Add constant: `logTagGeoBlock = "GEOBLOCK"`
3. Add field to `Server` struct: `geoFilter *geoblock.DB`
4. Add `WithGeoFilter` option function:

```go
// WithGeoFilter sets the GeoLite2 country blocking database for the
// broker server. When set, incoming connections are checked against
// the blocked country list before the handshake.
func WithGeoFilter(db *geoblock.DB) ServerOption {
	return func(s *Server) {
		s.geoFilter = db
	}
}
```

5. Add log method:

```go
// logGeoBlockedIP logs a connection that was rejected by the geo-block
// country filter. The remote address and the country code are logged.
func (s *Server) logGeoBlockedIP(remoteAddr, countryCode string) {
	if s.logger == nil {
		return
	}

	_ = s.logger.Warn(logTagGeoBlock,
		fmt.Sprintf("blocked connection from %s (country=%s)", remoteAddr, countryCode))
}
```

**Step 2: Add the geo-block check in acceptLoop**

In `acceptLoop()`, after the IPsum check block (lines 164-173) and before the semaphore check (lines 177-184), add:

```go
		// Check IP against the geo-block country filter before any protocol work.
		if s.geoFilter != nil {
			if ip, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
				if blocked, countryCode := s.geoFilter.IsBlocked(ip); blocked {
					s.logGeoBlockedIP(conn.RemoteAddr().String(), countryCode)
					conn.Close()
					continue
				}
			}
		}
```

**Step 3: Run all tests**

Run:
```bash
cd /workspace && go test ./... -timeout 60s
```

Expected: All tests PASS (including existing broker tests).

---

### Task 5: Add CLI flags to broker command

**Files:**
- Modify: `cmd/broker.go`

**Step 1: Add import and flags**

In `cmd/broker.go`:

1. Add import: `"github.com/mkloubert/go-connect/pkg/geoblock"`
2. Add two new flags to the command at the bottom (after `enable-ipsum`):

```go
	cmd.Flags().Bool("enable-geo-blocker", false, "enable GeoLite2 country blocking (overrides GO_CONNECT_ENABLE_GEO_BLOCKER env var)")
	cmd.Flags().String("blocked-countries", "", "comma-separated ISO country codes to block (overrides GO_CONNECT_BLOCKED_COUNTRIES env var)")
```

**Step 2: Add the geo-block setup logic in RunE**

In the `RunE` function, after the IPsum block (after `opts = append(opts, broker.WithIPFilter(ipFilter))`) and before `srv := broker.NewServer(...)`, add:

```go
			enableGeoBlocker, _ := cmd.Flags().GetBool("enable-geo-blocker")
			if !enableGeoBlocker && os.Getenv("GO_CONNECT_ENABLE_GEO_BLOCKER") == "1" {
				enableGeoBlocker = true
			}

			var geoDB *geoblock.DB
			if enableGeoBlocker {
				countriesStr, _ := cmd.Flags().GetString("blocked-countries")
				if countriesStr == "" {
					countriesStr = os.Getenv("GO_CONNECT_BLOCKED_COUNTRIES")
				}

				countries := strings.Split(countriesStr, ",")

				var err error
				geoDB, err = geoblock.Open(geoblock.DefaultDBPath, countries)
				if err != nil {
					out.Error(fmt.Sprintf("Failed to open GeoLite2 database: %v", err))
					out.Hint("Place a GeoLite2-City.mmdb file in the current directory.")
					return fmt.Errorf("geo-block database required but unavailable: %w", err)
				}
				defer geoDB.Close()
				out.Success(fmt.Sprintf("Geo-blocker loaded: %d countries blocked", len(geoDB.Countries())))
				opts = append(opts, broker.WithGeoFilter(geoDB))
			}
```

**Step 3: Verify compilation and run tests**

Run:
```bash
cd /workspace && go build ./... && go test ./... -timeout 60s
```

Expected: Clean build, all tests PASS.

---

## Phase 3: Documentation

### Task 6: Update TASKS.md with milestone checklist

**Files:**
- Modify: `TASKS.md`

**Step 1: Write the task checklist**

Update `TASKS.md` with:

```markdown
# Geo-Block Feature Tasks

## Phase 1: Core `geoblock` Package

- [x] Add `github.com/oschwald/geoip2-golang/v2` dependency
- [x] Download MaxMind test fixture `GeoIP2-City-Test.mmdb`
- [x] Write unit tests for `pkg/geoblock/`
- [x] Implement `pkg/geoblock/geoblock.go`

## Phase 2: Broker Integration

- [x] Add `WithGeoFilter` ServerOption and acceptLoop check in `server.go`
- [x] Add `--enable-geo-blocker` and `--blocked-countries` CLI flags in `broker.go`

## Phase 3: Documentation

- [x] Update TASKS.md checklist
- [x] Update README.md with geo-blocking docs
```

---

### Task 7: Update README.md

**Files:**
- Modify: `README.md`

**Step 1: Add geo-blocking section to README**

In the **Security** section of `README.md`, after the "IPsum threat intelligence filter" subsection, add a new subsection:

```markdown
### Geo-blocking (country filter)

The broker can block connections from specific countries using MaxMind's [GeoLite2-City](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) database. This feature is **disabled by default** and must be enabled with `--enable-geo-blocker` or `GO_CONNECT_ENABLE_GEO_BLOCKER=1`.

When enabled, the broker reads `GeoLite2-City.mmdb` from the current working directory. The file is **not** downloaded automatically -- you must obtain it from MaxMind.

Blocked countries are specified as a comma-separated list of ISO 3166-1 alpha-2 codes via `--blocked-countries` or `GO_CONNECT_BLOCKED_COUNTRIES`. Codes are trimmed and matched case-insensitively.

Example:

\```bash
# Block connections from Russia, China, and North Korea
./go-connect broker --enable-geo-blocker --blocked-countries="RU,CN,KP"
\```

Connections from blocked countries are rejected before the handshake, with no protocol overhead. Both IPv4 and IPv6 addresses are checked.

Blocked connections are logged with the `GEOBLOCK` tag in the security log.
```

Also update the following sections:

1. **Command reference table:** Add `--enable-geo-blocker`, `--blocked-countries` to broker's optional flags.
2. **Environment variables table:** Add `GO_CONNECT_ENABLE_GEO_BLOCKER` and `GO_CONNECT_BLOCKED_COUNTRIES`.
3. **Security logging table:** Add the GEOBLOCK row.
4. **Architecture tree:** Add `│   ├── geoblock/` line under `pkg/`.

**Step 2: Verify the final state**

Run:
```bash
cd /workspace && go build ./... && go test ./... -timeout 60s
```

Expected: Clean build, all tests PASS.
