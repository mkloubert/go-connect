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

import "testing"

func TestParseBrokerAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "127.0.0.1:1781"},
		{"whitespace only", "  ", "127.0.0.1:1781"},
		{"full address", "10.0.0.1:2000", "10.0.0.1:2000"},
		{"port only", ":2000", "127.0.0.1:2000"},
		{"host only", "10.0.0.1", "10.0.0.1:1781"},
		{"hostname only", "broker.example.com", "broker.example.com:1781"},
		{"hostname with port", "broker.example.com:3000", "broker.example.com:3000"},
		{"default host and port", "127.0.0.1:1781", "127.0.0.1:1781"},
		{"colon only", ":", "127.0.0.1:1781"},
		{"whitespace around full address", "  10.0.0.1:2000  ", "10.0.0.1:2000"},
		{"localhost", "localhost", "localhost:1781"},
		{"localhost with port", "localhost:9000", "localhost:9000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBrokerAddress(tt.input)
			if got != tt.want {
				t.Errorf("parseBrokerAddress(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseBindAddress(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", "0.0.0.0:1781"},
		{"whitespace only", "  ", "0.0.0.0:1781"},
		{"full address", "192.168.1.10:2000", "192.168.1.10:2000"},
		{"port only", ":2000", "0.0.0.0:2000"},
		{"host only", "192.168.1.10", "192.168.1.10:1781"},
		{"colon only", ":", "0.0.0.0:1781"},
		{"default bind address", "0.0.0.0:1781", "0.0.0.0:1781"},
		{"localhost with port", "127.0.0.1:9000", "127.0.0.1:9000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBindAddress(tt.input)
			if got != tt.want {
				t.Errorf("parseBindAddress(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
