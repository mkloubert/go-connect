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

package broker

import (
	"testing"
)

// newDummyClientConn creates a minimal ClientConn for router tests.
// It does not perform a handshake or open a real connection.
func newDummyClientConn(router *Router) *ClientConn {
	return &ClientConn{
		router:  router,
		closeCh: make(chan struct{}),
	}
}

func TestRouter_RegisterAndLookup(t *testing.T) {
	r := NewRouter()
	client := newDummyClientConn(r)

	const connID = "test-connection-id"

	if err := r.RegisterListener(connID, client); err != nil {
		t.Fatalf("RegisterListener returned unexpected error: %v", err)
	}

	found := r.FindListener(connID)
	if found != client {
		t.Fatalf("FindListener returned %v, want %v", found, client)
	}

	notFound := r.FindListener("nonexistent-id")
	if notFound != nil {
		t.Fatalf("FindListener for nonexistent ID returned %v, want nil", notFound)
	}
}

func TestRouter_RegisterDuplicate(t *testing.T) {
	r := NewRouter()
	client1 := newDummyClientConn(r)
	client2 := newDummyClientConn(r)

	const connID = "duplicate-id"

	if err := r.RegisterListener(connID, client1); err != nil {
		t.Fatalf("first RegisterListener returned unexpected error: %v", err)
	}

	err := r.RegisterListener(connID, client2)
	if err == nil {
		t.Fatal("second RegisterListener with same ID should return error, got nil")
	}
}

func TestRouter_LinkAndLookup(t *testing.T) {
	r := NewRouter()
	listener := newDummyClientConn(r)
	connector := newDummyClientConn(r)

	const connID = "link-test-id"

	if err := r.RegisterListener(connID, listener); err != nil {
		t.Fatalf("RegisterListener returned unexpected error: %v", err)
	}

	if err := r.LinkConnector(connID, connector); err != nil {
		t.Fatalf("LinkConnector returned unexpected error: %v", err)
	}

	// Verify bidirectional peer lookup.
	if peer := r.GetPeer(listener); peer != connector {
		t.Fatalf("GetPeer(listener) = %v, want connector %v", peer, connector)
	}
	if peer := r.GetPeer(connector); peer != listener {
		t.Fatalf("GetPeer(connector) = %v, want listener %v", peer, listener)
	}

	// Verify LinkConnector with nonexistent ID fails.
	err := r.LinkConnector("nonexistent-id", newDummyClientConn(r))
	if err == nil {
		t.Fatal("LinkConnector with nonexistent ID should return error, got nil")
	}
}

func TestRouter_LinkDuplicateConnector(t *testing.T) {
	r := NewRouter()
	listener := newDummyClientConn(r)
	connector1 := newDummyClientConn(r)
	connector2 := newDummyClientConn(r)

	const connID = "duplicate-connector-id"

	if err := r.RegisterListener(connID, listener); err != nil {
		t.Fatalf("RegisterListener returned unexpected error: %v", err)
	}

	if err := r.LinkConnector(connID, connector1); err != nil {
		t.Fatalf("first LinkConnector returned unexpected error: %v", err)
	}

	// A second connector to the same connection ID must be rejected.
	err := r.LinkConnector(connID, connector2)
	if err == nil {
		t.Fatal("second LinkConnector with same ID should return error, got nil")
	}

	// The first connector's peer link must remain intact.
	if peer := r.GetPeer(listener); peer != connector1 {
		t.Fatalf("GetPeer(listener) = %v, want original connector %v", peer, connector1)
	}
	if peer := r.GetPeer(connector1); peer != listener {
		t.Fatalf("GetPeer(connector1) = %v, want listener %v", peer, listener)
	}

	// The rejected connector must have no peer link.
	if peer := r.GetPeer(connector2); peer != nil {
		t.Fatalf("GetPeer(connector2) = %v, want nil", peer)
	}
}

func TestRouter_Remove(t *testing.T) {
	r := NewRouter()
	listener := newDummyClientConn(r)
	connector := newDummyClientConn(r)

	const connID = "remove-test-id"

	if err := r.RegisterListener(connID, listener); err != nil {
		t.Fatalf("RegisterListener returned unexpected error: %v", err)
	}
	if err := r.LinkConnector(connID, connector); err != nil {
		t.Fatalf("LinkConnector returned unexpected error: %v", err)
	}

	// Remove the listener.
	r.Remove(listener)

	// Listener should no longer be found.
	if found := r.FindListener(connID); found != nil {
		t.Fatalf("FindListener after Remove returned %v, want nil", found)
	}

	// Peer mappings should be cleaned up in both directions.
	if peer := r.GetPeer(listener); peer != nil {
		t.Fatalf("GetPeer(listener) after Remove = %v, want nil", peer)
	}
	if peer := r.GetPeer(connector); peer != nil {
		t.Fatalf("GetPeer(connector) after Remove = %v, want nil", peer)
	}
}
