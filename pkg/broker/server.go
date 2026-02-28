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
	"log"
	"net"
	"sync"

	pb "github.com/mkloubert/go-connect/pb"
)

// Server is the broker server that accepts client connections,
// performs handshakes, and relays messages between linked peers.
type Server struct {
	address  string
	router   *Router
	listener net.Listener
	clients  map[*ClientConn]struct{}
	clientMu sync.Mutex
	wg       sync.WaitGroup
	closeCh  chan struct{}
}

// NewServer creates a new broker Server that will listen on the given
// address (e.g., ":1781" or "127.0.0.1:0").
func NewServer(address string) *Server {
	return &Server{
		address: address,
		router:  NewRouter(),
		clients: make(map[*ClientConn]struct{}),
		closeCh: make(chan struct{}),
	}
}

// Start begins listening for incoming TCP connections and spawns
// the accept loop in a background goroutine.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.address, err)
	}

	s.listener = ln

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// acceptLoop accepts incoming connections and spawns a handler
// goroutine for each. It runs until the listener is closed.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closeCh:
				return
			default:
				log.Printf("broker: accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleClient(conn)
		}()
	}
}

// handleClient performs the handshake, reads the first message to
// determine if the client is a listener (Register) or a connector
// (ConnectRequest), and dispatches accordingly. If the first message
// is neither, the connection is closed with an error.
func (s *Server) handleClient(conn net.Conn) {
	client, err := NewClientConn(conn, s.router)
	if err != nil {
		log.Printf("broker: handshake failed: %v", err)
		conn.Close()
		return
	}

	s.trackClient(client)
	defer s.untrackClient(client)

	// Read first message to determine client role.
	env, err := client.Receive()
	if err != nil {
		log.Printf("broker: failed to receive first message: %v", err)
		client.Close()
		return
	}

	client.UpdateLastSeen()

	switch {
	case env.GetRegister() != nil:
		connectionID := env.GetRegister().GetConnectionId()
		s.handleListener(client, connectionID)

	case env.GetConnectRequest() != nil:
		connectionID := env.GetConnectRequest().GetConnectionId()
		s.handleConnector(client, connectionID)

	default:
		log.Printf("broker: unexpected first message type: %T", env.GetPayload())
		_ = client.Send(&pb.Envelope{
			Payload: &pb.Envelope_ErrorMsg{
				ErrorMsg: &pb.ErrorMsg{
					Message: "expected Register or ConnectRequest as first message",
				},
			},
		})
		client.Close()
	}
}

// handleListener registers the client as a listener and enters the
// relay loop. It sends a RegisterAck to confirm registration.
func (s *Server) handleListener(client *ClientConn, connectionID string) {
	client.SetConnectionID(connectionID)

	if err := s.router.RegisterListener(connectionID, client); err != nil {
		log.Printf("broker: failed to register listener %q: %v", connectionID, err)
		_ = client.Send(&pb.Envelope{
			Payload: &pb.Envelope_RegisterAck{
				RegisterAck: &pb.RegisterAck{
					Success: false,
					Message: err.Error(),
				},
			},
		})
		client.Close()
		return
	}

	_ = client.Send(&pb.Envelope{
		Payload: &pb.Envelope_RegisterAck{
			RegisterAck: &pb.RegisterAck{
				Success: true,
				Message: "registered",
			},
		},
	})

	client.StartHeartbeat()
	s.relayMessages(client)
}

// handleConnector links the client to a registered listener and enters
// the relay loop. It sends a ConnectAck to confirm the link.
func (s *Server) handleConnector(client *ClientConn, connectionID string) {
	client.SetConnectionID(connectionID)

	if err := s.router.LinkConnector(connectionID, client); err != nil {
		log.Printf("broker: failed to link connector %q: %v", connectionID, err)
		_ = client.Send(&pb.Envelope{
			Payload: &pb.Envelope_ConnectAck{
				ConnectAck: &pb.ConnectAck{
					Success: false,
					Message: err.Error(),
				},
			},
		})
		client.Close()
		return
	}

	_ = client.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectAck{
			ConnectAck: &pb.ConnectAck{
				Success: true,
				Message: "connected",
			},
		},
	})

	client.StartHeartbeat()
	s.relayMessages(client)
}

// relayMessages continuously receives messages from the client and
// forwards them to the linked peer. Heartbeat messages are consumed
// locally and not forwarded. The loop exits when a receive error
// occurs (e.g., connection closed).
func (s *Server) relayMessages(client *ClientConn) {
	for {
		env, err := client.Receive()
		if err != nil {
			client.Close()
			return
		}

		client.UpdateLastSeen()

		// Heartbeat messages are consumed by the broker; do not forward.
		if env.GetHeartbeat() != nil {
			continue
		}

		// Forward to peer.
		peer := s.router.GetPeer(client)
		if peer == nil {
			// No peer linked; close the client.
			client.Close()
			return
		}

		if err := peer.Send(env); err != nil {
			log.Printf("broker: failed to forward message to peer: %v", err)
			client.Close()
			return
		}
	}
}

// trackClient adds a client to the server's active set.
func (s *Server) trackClient(client *ClientConn) {
	s.clientMu.Lock()
	s.clients[client] = struct{}{}
	s.clientMu.Unlock()
}

// untrackClient removes a client from the server's active set.
func (s *Server) untrackClient(client *ClientConn) {
	s.clientMu.Lock()
	delete(s.clients, client)
	s.clientMu.Unlock()
}

// Stop gracefully shuts down the broker server. It closes the listener
// to stop accepting new connections, closes all active client connections
// to unblock handler goroutines, and waits for them to complete.
func (s *Server) Stop() {
	close(s.closeCh)

	if s.listener != nil {
		s.listener.Close()
	}

	// Close all active client connections to unblock relayMessages loops.
	s.clientMu.Lock()
	for client := range s.clients {
		client.Close()
	}
	s.clientMu.Unlock()

	s.wg.Wait()
}

// Address returns the actual network address the server is listening
// on. This is useful when the server is started on port 0 to get
// the dynamically assigned port.
func (s *Server) Address() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.address
}
