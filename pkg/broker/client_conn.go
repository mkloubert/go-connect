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
	"net"
	"sync"
	"time"

	pb "github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

const (
	// HeartbeatInterval is the interval between heartbeat messages sent
	// by the broker to connected clients.
	HeartbeatInterval = 15 * time.Second

	// HeartbeatTimeout is the maximum duration without receiving any
	// message from a client before the connection is considered stale
	// and closed.
	HeartbeatTimeout = 45 * time.Second
)

// ClientConn represents a single client connection to the broker.
// It wraps a net.Conn with an encrypted protocol.Session and provides
// lifecycle management including heartbeat monitoring.
type ClientConn struct {
	conn         net.Conn
	session      *protocol.Session
	connectionID string
	router       *Router
	lastSeen     time.Time
	closeCh      chan struct{}
	closeOnce    sync.Once
	mu           sync.Mutex
}

// NewClientConn performs a handshake on the given connection and returns
// a new ClientConn ready for encrypted communication.
func NewClientConn(conn net.Conn, router *Router) (*ClientConn, error) {
	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		return nil, err
	}

	return &ClientConn{
		conn:     conn,
		session:  session,
		router:   router,
		lastSeen: time.Now(),
		closeCh:  make(chan struct{}),
	}, nil
}

// Send sends an encrypted protobuf Envelope to the client.
func (cc *ClientConn) Send(env *pb.Envelope) error {
	return cc.session.Send(env)
}

// Receive reads and decrypts a protobuf Envelope from the client.
func (cc *ClientConn) Receive() (*pb.Envelope, error) {
	return cc.session.Receive()
}

// SetConnectionID sets the connection ID for this client.
func (cc *ClientConn) SetConnectionID(id string) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.connectionID = id
}

// ConnectionID returns the connection ID for this client.
func (cc *ClientConn) ConnectionID() string {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	return cc.connectionID
}

// UpdateLastSeen updates the last-seen timestamp to the current time.
func (cc *ClientConn) UpdateLastSeen() {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.lastSeen = time.Now()
}

// LastSeen returns the last-seen timestamp.
func (cc *ClientConn) LastSeen() time.Time {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	return cc.lastSeen
}

// Close shuts down the client connection. It is safe to call multiple
// times; only the first call performs cleanup. The close sequence:
//  1. Close the done channel to signal goroutines.
//  2. Close the underlying network connection.
//  3. Get peer reference before removing self from router.
//  4. Remove self from router to break peer link.
//  5. If a peer exists, notify it with a Disconnect message and close it.
//
// The closeOnce ensures no infinite recursion: when peer.Close() is
// called, it will try to get its peer (us), but we are already removed
// from the router so GetPeer returns nil.
func (cc *ClientConn) Close() {
	var peer *ClientConn

	cc.closeOnce.Do(func() {
		// 1. Signal done.
		close(cc.closeCh)

		// 2. Close network connection.
		if cc.conn != nil {
			cc.conn.Close()
		}

		// 3. Get peer before removing from router.
		if cc.router != nil {
			peer = cc.router.GetPeer(cc)
		}

		// 4. Remove self from router (breaks peer link so the peer's
		//    Close will not find us via GetPeer, preventing recursion).
		if cc.router != nil {
			cc.router.Remove(cc)
		}
	})

	// 5. Notify and close peer outside the Once block to avoid a
	//    deadlock: if both peers call Close concurrently, each would
	//    hold their own Once mutex while trying to acquire the other's.
	if peer != nil {
		// Best-effort: send Disconnect message to peer.
		_ = peer.Send(&pb.Envelope{
			Payload: &pb.Envelope_Disconnect{
				Disconnect: &pb.Disconnect{
					Reason: "peer disconnected",
				},
			},
		})
		peer.Close()
	}
}

// Done returns a channel that is closed when the client connection
// is shut down.
func (cc *ClientConn) Done() <-chan struct{} {
	return cc.closeCh
}

// StartHeartbeat launches a goroutine that periodically checks client
// liveness and sends heartbeat messages. If the client has not been
// seen within HeartbeatTimeout, the connection is closed. Otherwise,
// a Heartbeat message is sent every HeartbeatInterval. The goroutine
// exits when the client's Done channel is closed.
func (cc *ClientConn) StartHeartbeat() {
	go func() {
		ticker := time.NewTicker(HeartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cc.Done():
				return
			case <-ticker.C:
				if time.Since(cc.LastSeen()) > HeartbeatTimeout {
					cc.Close()
					return
				}

				_ = cc.Send(&pb.Envelope{
					Payload: &pb.Envelope_Heartbeat{
						Heartbeat: &pb.Heartbeat{},
					},
				})
			}
		}
	}()
}
