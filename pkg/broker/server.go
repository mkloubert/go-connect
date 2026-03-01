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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	pb "github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/geoblock"
	"github.com/mkloubert/go-connect/pkg/ipsum"
	"github.com/mkloubert/go-connect/pkg/logging"
)

const (
	// DefaultMaxConnections is the maximum number of concurrent client
	// connections the broker will accept. Connections beyond this limit
	// are rejected immediately, before the expensive handshake.
	DefaultMaxConnections = 1024

	// AuthTimeout is the maximum time the broker waits for a client
	// to send the Authenticate message after the handshake.
	AuthTimeout = 10 * time.Second

	// maxLogPayloadBytes is the maximum number of raw payload bytes
	// that are logged as Base64 when an invalid auth payload is received.
	maxLogPayloadBytes = 256

	// logTagAuth is the log tag for authentication-related events.
	logTagAuth = "AUTH"

	// logTagRouting is the log tag for connection routing events.
	logTagRouting = "ROUTING"

	// logTagIPBlock is the log tag for IP filter events.
	logTagIPBlock = "IPBLOCK"

	// logTagGeoBlock is the log tag for geo-block filter events.
	logTagGeoBlock = "GEOBLOCK"
)

// ServerOption configures optional Server parameters.
type ServerOption func(*Server)

// WithPassphrase sets the passphrase that clients must provide
// after the encrypted handshake.
func WithPassphrase(passphrase string) ServerOption {
	return func(s *Server) {
		hash := sha256.Sum256([]byte(passphrase))
		s.passphraseHash = hash[:]
	}
}

// WithLogger sets the security logger for the broker server.
// When set, security-relevant events such as invalid authentication
// payloads, passphrase mismatches, and invalid connection IDs are
// written to the log files.
func WithLogger(logger *logging.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithIPFilter sets the IPsum threat intelligence database for the
// broker server. When set, incoming connections are checked against
// the database before the handshake. Blocked IPs are silently
// disconnected and logged.
func WithIPFilter(db *ipsum.DB) ServerOption {
	return func(s *Server) {
		s.ipFilter = db
	}
}

// WithGeoFilter sets the GeoLite2 country blocking database for the
// broker server. When set, incoming connections are checked against
// the blocked country list before the handshake.
func WithGeoFilter(db *geoblock.DB) ServerOption {
	return func(s *Server) {
		s.geoFilter = db
	}
}

// Server is the broker server that accepts client connections,
// performs handshakes, and relays messages between linked peers.
type Server struct {
	address        string
	router         *Router
	listener       net.Listener
	clients        map[*ClientConn]struct{}
	clientMu       sync.Mutex
	connSem        chan struct{} // semaphore limiting concurrent connections
	wg             sync.WaitGroup
	closeCh        chan struct{}
	passphraseHash []byte
	logger         *logging.Logger
	ipFilter       *ipsum.DB
	geoFilter      *geoblock.DB
}

// NewServer creates a new broker Server that will listen on the given
// address (e.g., ":1781" or "127.0.0.1:0").
func NewServer(address string, opts ...ServerOption) *Server {
	s := &Server{
		address: address,
		router:  NewRouter(),
		clients: make(map[*ClientConn]struct{}),
		connSem: make(chan struct{}, DefaultMaxConnections),
		closeCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.passphraseHash == nil {
		hash := sha256.Sum256([]byte(""))
		s.passphraseHash = hash[:]
	}
	return s
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
// Connections are rejected when the server is at capacity.
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

		// Check IP against the IPsum blacklist before any protocol work.
		if s.ipFilter != nil {
			if ip, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
				if blocked, count := s.ipFilter.IsBlocked(ip); blocked {
					s.logBlockedIP(conn.RemoteAddr().String(), count)
					conn.Close()
					continue
				}
			}
		}

		// Check IP against the geo-block country filter before any protocol work.
		if s.geoFilter != nil {
			if ip, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
				if blocked, countryCode := s.geoFilter.IsBlocked(ip); blocked {
					s.logGeoBlockedIP(conn.RemoteAddr().String(), countryCode)
					conn.Close()
					continue
				}
			}
		}

		// Reject the connection immediately if at capacity,
		// before the expensive handshake.
		select {
		case s.connSem <- struct{}{}:
			// Slot acquired.
		default:
			log.Printf("broker: connection limit reached, rejecting connection from %s", conn.RemoteAddr())
			conn.Close()
			continue
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-s.connSem }()
			s.handleClient(conn)
		}()
	}
}

// authenticateClient reads the Authenticate message from the client
// and validates the passphrase hash using constant-time comparison.
// The read is bounded by AuthTimeout; if the client does not respond
// in time, the connection is closed.
//
// Security logging:
//   - Invalid payload (not an Authenticate message): logged with raw
//     content as Base64 (max 256 bytes), remote IP and port.
//   - Wrong passphrase: logged with remote IP and port only.
func (s *Server) authenticateClient(client *ClientConn) error {
	if err := client.conn.SetReadDeadline(time.Now().Add(AuthTimeout)); err != nil {
		client.conn.Close()
		return fmt.Errorf("failed to set auth deadline: %w", err)
	}

	rawBytes, env, err := client.ReceiveRaw()
	if err != nil {
		s.logInvalidAuthPayload(client, rawBytes)
		client.conn.Close()
		return fmt.Errorf("failed to receive auth message: %w", err)
	}

	if err := client.conn.SetReadDeadline(time.Time{}); err != nil {
		client.conn.Close()
		return fmt.Errorf("failed to clear auth deadline: %w", err)
	}

	auth := env.GetAuthenticate()
	if auth == nil {
		s.logInvalidAuthPayload(client, rawBytes)
		client.conn.Close()
		return fmt.Errorf("expected Authenticate, got %T", env.GetPayload())
	}

	if subtle.ConstantTimeCompare(auth.GetPassphraseHash(), s.passphraseHash) != 1 {
		s.logPassphraseMismatch(client)
		client.conn.Close()
		return fmt.Errorf("passphrase mismatch")
	}

	return nil
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

	// Authenticate the client before determining its role.
	if err := s.authenticateClient(client); err != nil {
		log.Printf("broker: authentication failed: %v", err)
		return
	}

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
		s.logInvalidConnectionID(client, connectionID)
		_ = client.Send(&pb.Envelope{
			Payload: &pb.Envelope_RegisterAck{
				RegisterAck: &pb.RegisterAck{
					Success: false,
					Message: "registration failed",
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
		s.logInvalidConnectionID(client, connectionID)
		_ = client.Send(&pb.Envelope{
			Payload: &pb.Envelope_ConnectAck{
				ConnectAck: &pb.ConnectAck{
					Success: false,
					Message: "connection failed",
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

// logInvalidAuthPayload logs an invalid authentication payload. This
// detects clients (e.g., bots) that do not send a proper Authenticate
// message after the handshake. The raw content is logged as Base64,
// truncated to maxLogPayloadBytes.
func (s *Server) logInvalidAuthPayload(client *ClientConn, rawBytes []byte) {
	if s.logger == nil {
		return
	}

	payload := rawBytes
	if len(payload) > maxLogPayloadBytes {
		payload = payload[:maxLogPayloadBytes]
	}

	encoded := base64.StdEncoding.EncodeToString(payload)
	remote := client.RemoteAddr().String()

	_ = s.logger.Warn(logTagAuth,
		fmt.Sprintf("invalid auth payload from %s: %s", remote, encoded))
}

// logPassphraseMismatch logs a failed passphrase authentication attempt.
// Only the remote address is logged; the submitted passphrase value is
// never written to logs.
func (s *Server) logPassphraseMismatch(client *ClientConn) {
	if s.logger == nil {
		return
	}

	remote := client.RemoteAddr().String()

	_ = s.logger.Warn(logTagAuth,
		fmt.Sprintf("passphrase mismatch from %s", remote))
}

// logInvalidConnectionID logs an attempt to use an invalid connection ID.
// The remote address and the submitted connection ID value are logged.
func (s *Server) logInvalidConnectionID(client *ClientConn, connectionID string) {
	if s.logger == nil {
		return
	}

	remote := client.RemoteAddr().String()

	_ = s.logger.Warn(logTagRouting,
		fmt.Sprintf("invalid connection ID %q from %s", connectionID, remote))
}

// logBlockedIP logs a connection that was rejected by the IPsum IP filter.
// The remote address (IP:port) and the blacklist count are logged.
func (s *Server) logBlockedIP(remoteAddr string, count int) {
	if s.logger == nil {
		return
	}

	_ = s.logger.Warn(logTagIPBlock,
		fmt.Sprintf("blocked connection from %s (ipsum count=%d)", remoteAddr, count))
}

// logGeoBlockedIP logs a connection that was rejected by the geo-block
// country filter. The remote address and the country code are logged.
func (s *Server) logGeoBlockedIP(remoteAddr, countryCode string) {
	if s.logger == nil {
		return
	}

	_ = s.logger.Warn(logTagGeoBlock,
		fmt.Sprintf("blocked connection from %s (country=%s)", remoteAddr, countryCode))
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
