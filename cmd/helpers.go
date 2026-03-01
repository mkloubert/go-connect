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

package cmd

import (
	"net"
	"strings"
)

const defaultPort = "1781"

// parseAddress parses an address string into a normalized host:port form
// using the given defaults. It supports the following input formats:
//   - "host:port" -> "host:port"
//   - ":port"     -> "defaultHost:port"
//   - "host"      -> "host:defaultPort"
//   - ""          -> "defaultHost:defaultPort"
func parseAddress(addr, defaultHost string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return net.JoinHostPort(defaultHost, defaultPort)
	}

	if strings.HasPrefix(addr, ":") {
		port := strings.TrimPrefix(addr, ":")
		if port == "" {
			port = defaultPort
		}
		return net.JoinHostPort(defaultHost, port)
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}

	if host == "" {
		host = defaultHost
	}
	if port == "" {
		port = defaultPort
	}

	return net.JoinHostPort(host, port)
}

// parseBrokerAddress parses a broker address for --broker flags.
// Default host: 127.0.0.1, default port: 1781.
func parseBrokerAddress(addr string) string {
	return parseAddress(addr, "127.0.0.1")
}

// parseBindAddress parses a bind address for --bind-to flags.
// Default host: 0.0.0.0, default port: 1781.
func parseBindAddress(addr string) string {
	return parseAddress(addr, "0.0.0.0")
}
