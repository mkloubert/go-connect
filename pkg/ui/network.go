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
	"fmt"
	"net"
)

// InterfaceAddr holds information about a single network interface address.
type InterfaceAddr struct {
	IP         string
	Port       string
	Iface      string
	IsLoopback bool
	IsIPv6     bool
}

// Display returns the address formatted for display.
// IPv4 addresses are returned as "IP:Port", IPv6 as "[IP]:Port".
func (a InterfaceAddr) Display() string {
	if a.IsIPv6 {
		return fmt.Sprintf("[%s]:%s", a.IP, a.Port)
	}

	return fmt.Sprintf("%s:%s", a.IP, a.Port)
}

// Label returns a human-readable label for the interface.
// Loopback interfaces get ", local only" appended;
// IPv6 addresses get ", IPv6" appended;
// otherwise just the interface name is returned.
func (a InterfaceAddr) Label() string {
	if a.IsLoopback {
		return a.Iface + ", local only"
	}

	if a.IsIPv6 {
		return a.Iface + ", IPv6"
	}

	return a.Iface
}

// ListAddresses enumerates all UP network interfaces and returns their
// unicast addresses paired with the given port.
// Non-loopback addresses are listed first, loopback addresses last.
// Returns an empty slice if an error occurs while reading interfaces.
func ListAddresses(port string) []InterfaceAddr {
	ifaces, err := net.Interfaces()
	if err != nil {
		return []InterfaceAddr{}
	}

	var nonLoopback []InterfaceAddr
	var loopback []InterfaceAddr

	for _, iface := range ifaces {
		// Skip interfaces that are not up.
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		isLoopback := iface.Flags&net.FlagLoopback != 0

		for _, addr := range addrs {
			var ip net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			// Skip unspecified and multicast addresses.
			if ip.IsUnspecified() || ip.IsMulticast() {
				continue
			}

			entry := InterfaceAddr{
				IP:         ip.String(),
				Port:       port,
				Iface:      iface.Name,
				IsLoopback: isLoopback,
				IsIPv6:     ip.To4() == nil,
			}

			if isLoopback {
				loopback = append(loopback, entry)
			} else {
				nonLoopback = append(nonLoopback, entry)
			}
		}
	}

	return append(nonLoopback, loopback...)
}
