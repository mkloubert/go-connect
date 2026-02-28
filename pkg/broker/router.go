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
	"fmt"
	"sync"
)

// Router manages listener registrations and bidirectional peer mappings
// between connected clients. It is safe for concurrent use.
type Router struct {
	listeners map[string]*ClientConn
	peers     map[*ClientConn]*ClientConn
	mu        sync.RWMutex
}

// NewRouter creates a new Router with initialized internal maps.
func NewRouter() *Router {
	return &Router{
		listeners: make(map[string]*ClientConn),
		peers:     make(map[*ClientConn]*ClientConn),
	}
}

// RegisterListener registers a listener client under the given connectionID.
// Returns an error if the connectionID is already registered.
func (r *Router) RegisterListener(connectionID string, client *ClientConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.listeners[connectionID]; exists {
		return fmt.Errorf("connection ID %q is already registered", connectionID)
	}

	r.listeners[connectionID] = client
	return nil
}

// FindListener looks up a registered listener by its connectionID.
// Returns the ClientConn if found, or nil if no listener is registered
// under the given ID.
func (r *Router) FindListener(connectionID string) *ClientConn {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.listeners[connectionID]
}

// LinkConnector finds the listener for the given connectionID and creates
// a bidirectional peer mapping between the listener and the connector.
// Returns an error if no listener is found for the given connectionID.
func (r *Router) LinkConnector(connectionID string, connector *ClientConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	listener, exists := r.listeners[connectionID]
	if !exists {
		return fmt.Errorf("no listener registered for connection ID %q", connectionID)
	}

	// Reject if a connector is already linked to this listener.
	if _, linked := r.peers[listener]; linked {
		return fmt.Errorf("connection ID %q already has a linked connector", connectionID)
	}

	r.peers[listener] = connector
	r.peers[connector] = listener

	return nil
}

// GetPeer returns the peer for the given client, or nil if no peer
// mapping exists.
func (r *Router) GetPeer(client *ClientConn) *ClientConn {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.peers[client]
}

// Remove removes the given client from all router data structures.
// It removes the client from the listeners map (if registered) and
// removes the bidirectional peer mapping (if linked).
func (r *Router) Remove(client *ClientConn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from listeners map.
	for id, c := range r.listeners {
		if c == client {
			delete(r.listeners, id)
			break
		}
	}

	// Remove bidirectional peer mapping.
	if peer, exists := r.peers[client]; exists {
		delete(r.peers, peer)
		delete(r.peers, client)
	}
}
