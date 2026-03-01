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
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	// DefaultURL is the default URL for the IPsum threat intelligence feed.
	DefaultURL = "https://raw.githubusercontent.com/stamparm/ipsum/master/ipsum.txt"

	// DefaultFilePath is the default local file name for the downloaded feed.
	DefaultFilePath = "ipsum.txt"

	// DefaultMinCount is the default minimum blacklist count threshold.
	// IPs with a count equal to or above this value are blocked.
	DefaultMinCount = 3

	// initialMapCapacity is the initial capacity hint for the IP map.
	// IPsum typically contains ~150k entries; pre-allocating avoids rehashing.
	initialMapCapacity = 200_000

	// downloadFilePermissions is the permission mode for the downloaded file.
	downloadFilePermissions = 0644
)

// WarnFunc is called for each unparseable line encountered during loading.
// lineNum is the 1-based line number, line is the raw content.
type WarnFunc func(lineNum int, line string)

// DB holds a parsed IPsum feed and provides fast IP lookups.
// Only IPs meeting the minimum blacklist count are stored to save memory.
type DB struct {
	ips      map[string]int
	minCount int
}

// NewDB creates a new empty IPsum database with the given minimum
// blacklist count threshold. IPs with a count below minCount are
// not stored and are never reported as blocked.
func NewDB(minCount int) *DB {
	if minCount < 1 {
		minCount = 1
	}
	return &DB{
		ips:      make(map[string]int),
		minCount: minCount,
	}
}

// DownloadToFile downloads the IPsum feed from the given URL and saves
// it to the specified local file path. The file is written atomically
// by first writing to a temporary file and then renaming it. The HTTP
// request respects the provided context for cancellation and timeouts.
func DownloadToFile(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("ipsum: failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "go-connect/ipsum-filter")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ipsum: download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ipsum: unexpected HTTP status %d", resp.StatusCode)
	}

	tmpPath := path + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, downloadFilePermissions)
	if err != nil {
		return fmt.Errorf("ipsum: failed to create temp file %q: %w", tmpPath, err)
	}

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("ipsum: failed to write temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ipsum: failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("ipsum: failed to rename temp file to %q: %w", path, err)
	}

	return nil
}

// LoadFromFile reads and parses an IPsum feed from the given local file.
// Any previously loaded data is replaced. Only IPs with a blacklist
// count >= minCount are stored. Unparseable non-comment, non-empty lines
// are reported via the warn callback (if non-nil) but do not cause an error.
func (db *DB) LoadFromFile(path string, warn WarnFunc) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("ipsum: failed to open %q: %w", path, err)
	}
	defer f.Close()

	return db.loadFromReader(f, warn)
}

// LoadFromReader parses an IPsum feed from the given reader and populates
// the database. Any previously loaded data is replaced. Only IPs with a
// blacklist count >= minCount are stored. Unparseable lines are reported
// via the warn callback (if non-nil).
func (db *DB) LoadFromReader(r io.Reader, warn WarnFunc) error {
	return db.loadFromReader(r, warn)
}

func (db *DB) loadFromReader(r io.Reader, warn WarnFunc) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 5*1024*1024)

	newMap := make(map[string]int, initialMapCapacity)
	lineNum := 0

	for sc.Scan() {
		lineNum++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			if warn != nil {
				warn(lineNum, line)
			}
			continue
		}

		ip := net.ParseIP(fields[0])
		if ip == nil {
			if warn != nil {
				warn(lineNum, line)
			}
			continue
		}
		ip4 := ip.To4()
		if ip4 == nil {
			if warn != nil {
				warn(lineNum, line)
			}
			continue
		}

		cnt, err := strconv.Atoi(fields[1])
		if err != nil {
			if warn != nil {
				warn(lineNum, line)
			}
			continue
		}

		if cnt >= db.minCount {
			newMap[ip4.String()] = cnt
		}
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("ipsum: scan error: %w", err)
	}

	db.ips = newMap
	return nil
}

// IsBlocked reports whether the given IP should be blocked. It returns
// the blocked status and the blacklist count from the IPsum feed. For
// IPs not in the database, count is 0 and blocked is false.
func (db *DB) IsBlocked(ipStr string) (blocked bool, count int) {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false, 0
	}

	ip4 := ip.To4()
	if ip4 == nil {
		return false, 0
	}

	c, exists := db.ips[ip4.String()]
	if !exists {
		return false, 0
	}

	return true, c
}

// Len returns the number of IPs stored in the database.
func (db *DB) Len() int {
	return len(db.ips)
}
