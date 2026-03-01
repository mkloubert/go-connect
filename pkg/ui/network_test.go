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
	"testing"
)

func TestListAddresses_ReturnsAtLeastLoopback(t *testing.T) {
	addrs := ListAddresses("9999")

	if len(addrs) == 0 {
		t.Fatal("expected at least one address, got none")
	}

	foundLoopback := false
	for _, a := range addrs {
		if a.IsLoopback {
			foundLoopback = true
			break
		}
	}

	if !foundLoopback {
		t.Error("expected at least one loopback address to be present and marked")
	}
}

func TestListAddresses_PortAppended(t *testing.T) {
	port := "4242"
	addrs := ListAddresses(port)

	for _, a := range addrs {
		if a.Port != port {
			t.Errorf("expected port %q, got %q for address %s on %s", port, a.Port, a.IP, a.Iface)
		}
	}
}

func TestInterfaceAddr_DisplayFormat(t *testing.T) {
	a := InterfaceAddr{
		IP:     "192.168.1.10",
		Port:   "8080",
		Iface:  "eth0",
		IsIPv6: false,
	}

	got := a.Display()
	want := "192.168.1.10:8080"

	if got != want {
		t.Errorf("Display() = %q, want %q", got, want)
	}
}

func TestInterfaceAddr_DisplayFormat_IPv6(t *testing.T) {
	a := InterfaceAddr{
		IP:     "fe80::1",
		Port:   "8080",
		Iface:  "eth0",
		IsIPv6: true,
	}

	got := a.Display()
	want := "[fe80::1]:8080"

	if got != want {
		t.Errorf("Display() = %q, want %q", got, want)
	}
}

func TestInterfaceAddr_Label_Loopback(t *testing.T) {
	a := InterfaceAddr{
		IP:         "127.0.0.1",
		Port:       "8080",
		Iface:      "lo",
		IsLoopback: true,
		IsIPv6:     false,
	}

	got := a.Label()
	want := "lo, local only"

	if got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestInterfaceAddr_Label_IPv6(t *testing.T) {
	a := InterfaceAddr{
		IP:         "fe80::1",
		Port:       "8080",
		Iface:      "eth0",
		IsLoopback: false,
		IsIPv6:     true,
	}

	got := a.Label()
	want := "eth0, IPv6"

	if got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestInterfaceAddr_Label_Normal(t *testing.T) {
	a := InterfaceAddr{
		IP:         "192.168.1.10",
		Port:       "8080",
		Iface:      "eth0",
		IsLoopback: false,
		IsIPv6:     false,
	}

	got := a.Label()
	want := "eth0"

	if got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}
