# Geo-Block Feature Design

## Goal

Add optional country-based IP blocking to the broker using MaxMind's GeoLite2-City database. This allows blocking connections from specific countries before the handshake, similar to the existing IPsum filter.

## Activation

- Flag: `--enable-geo-blocker` (broker command)
- Env: `GO_CONNECT_ENABLE_GEO_BLOCKER=1`
- The `GeoLite2-City.mmdb` file must exist in the current working directory. It is NOT auto-downloaded.

## Country List

- Flag: `--blocked-countries` (broker command)
- Env: `GO_CONNECT_BLOCKED_COUNTRIES`
- Format: comma-separated ISO 3166-1 alpha-2 country codes (e.g., `"RU,CN,KP"`)
- Matching: trimmed and case-insensitive

## Architecture

### New Package: `pkg/geoblock/`

File: `pkg/geoblock/geoblock.go`

- `DB` struct wrapping `*geoip2.Reader` and a `map[string]struct{}` of blocked country codes (stored uppercase)
- `Open(dbPath string, countries []string) (*DB, error)` - opens the mmdb file and normalizes country codes
- `IsBlocked(ipStr string) (blocked bool, countryCode string)` - parses IP via `net/netip.ParseAddr`, performs City lookup, checks `Country.ISOCode` against the blocked set
- `Close() error` - closes the geoip2 reader
- Supports both IPv4 and IPv6
- Constant: `DefaultDBPath = "GeoLite2-City.mmdb"`

### Server Integration: `pkg/broker/server.go`

- New `ServerOption`: `WithGeoFilter(db *geoblock.DB)`
- New field: `geoFilter *geoblock.DB` on `Server` struct
- New log tag: `GEOBLOCK`
- New method: `logGeoBlockedIP(remoteAddr, countryCode string)`
- Check location: in `acceptLoop()`, after IPsum check, before semaphore

### CLI Integration: `cmd/broker.go`

- New flag: `--enable-geo-blocker` (bool, default false)
- New flag: `--blocked-countries` (string, default "")
- Env vars: `GO_CONNECT_ENABLE_GEO_BLOCKER`, `GO_CONNECT_BLOCKED_COUNTRIES`
- Validates: mmdb file exists when enabled, at least one country code provided
- Passes `broker.WithGeoFilter(db)` to server options
- Closes DB on shutdown via deferred call

### Logging

Format matches existing security log entries:

```
[20260301 14:23:45.123] [GEOBLOCK] [WARN]  blocked connection from 203.0.113.5:54321 (country=CN)
```

### Tests: `pkg/geoblock/geoblock_test.go`

- Country code normalization (trimming, case-insensitivity)
- Blocked vs. allowed IPs
- Missing database file error handling
- Empty country list behavior

### Dependency

- `github.com/oschwald/geoip2-golang/v2` (latest version)

## Documentation Updates

- README.md: new "Geo-blocking" section under Security
- Environment variable table: two new entries
- Command reference table: new flags for broker
