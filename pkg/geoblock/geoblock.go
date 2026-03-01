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
	"net/netip"
	"strings"

	"github.com/oschwald/geoip2-golang/v2"
)

// DefaultDBPath is the default file path for the MaxMind GeoLite2 City database.
const DefaultDBPath = "GeoLite2.mmdb"

// DB holds an opened MaxMind database reader and a set of blocked country codes.
type DB struct {
	reader  *geoip2.Reader
	blocked map[string]struct{}
}

// Open opens the MaxMind database at dbPath and normalizes the given country
// codes (trimmed whitespace, uppercased) into a lookup map. The caller must
// call Close when the DB is no longer needed.
func Open(dbPath string, countries []string) (*DB, error) {
	reader, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, err
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

// IsBlocked checks whether the given IP address belongs to a blocked country.
// It returns whether the IP is blocked and the resolved ISO country code.
// If the IP cannot be parsed, has no geo data, or is not in the blocked list,
// blocked is false. For parse errors or missing data, countryCode is "".
func (db *DB) IsBlocked(ipStr string) (blocked bool, countryCode string) {
	ipStr = strings.TrimSpace(ipStr)

	addr, err := netip.ParseAddr(ipStr)
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

	_, isBlocked := db.blocked[code]
	return isBlocked, code
}

// Countries returns the list of blocked country codes (normalized to uppercase).
func (db *DB) Countries() []string {
	codes := make([]string, 0, len(db.blocked))
	for code := range db.blocked {
		codes = append(codes, code)
	}
	return codes
}

// Close closes the underlying MaxMind database reader.
func (db *DB) Close() error {
	return db.reader.Close()
}
