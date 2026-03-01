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
	"bytes"
	"crypto/sha256"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pb "github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/ipsum"
	"github.com/mkloubert/go-connect/pkg/logging"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

// dialAndHandshake connects to the broker at the given address, performs
// the handshake, and sends an Authenticate message with an empty passphrase.
func dialAndHandshake(t *testing.T, addr string) (net.Conn, *protocol.Session) {
	return dialAndHandshakeWithPassphrase(t, addr, "")
}

// dialAndHandshakeWithPassphrase connects to the broker, performs the
// handshake, and sends an Authenticate message with the given passphrase.
func dialAndHandshakeWithPassphrase(t *testing.T, addr string, passphrase string) (net.Conn, *protocol.Session) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to dial broker at %s: %v", addr, err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		t.Fatalf("handshake failed: %v", err)
	}

	hash := sha256.Sum256([]byte(passphrase))
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	}); err != nil {
		conn.Close()
		t.Fatalf("failed to send Authenticate: %v", err)
	}

	return conn, session
}

func TestServer_ListenerConnectorLink(t *testing.T) {
	// Start broker on a random port.
	srv := NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	const connectionID = "integration-test-id"

	// --- Listener ---
	listenerConn, listenerSession := dialAndHandshake(t, addr)
	defer listenerConn.Close()

	// Send Register message.
	if err := listenerSession.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: connectionID,
			},
		},
	}); err != nil {
		t.Fatalf("listener: failed to send Register: %v", err)
	}

	// Receive RegisterAck.
	ack, err := listenerSession.Receive()
	if err != nil {
		t.Fatalf("listener: failed to receive RegisterAck: %v", err)
	}
	regAck := ack.GetRegisterAck()
	if regAck == nil {
		t.Fatalf("listener: expected RegisterAck, got %T", ack.GetPayload())
	}
	if !regAck.GetSuccess() {
		t.Fatalf("listener: RegisterAck not successful: %s", regAck.GetMessage())
	}

	// --- Connector ---
	connectorConn, connectorSession := dialAndHandshake(t, addr)
	defer connectorConn.Close()

	// Send ConnectRequest.
	if err := connectorSession.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectRequest{
			ConnectRequest: &pb.ConnectRequest{
				ConnectionId: connectionID,
			},
		},
	}); err != nil {
		t.Fatalf("connector: failed to send ConnectRequest: %v", err)
	}

	// Receive ConnectAck.
	cAck, err := connectorSession.Receive()
	if err != nil {
		t.Fatalf("connector: failed to receive ConnectAck: %v", err)
	}
	connectAck := cAck.GetConnectAck()
	if connectAck == nil {
		t.Fatalf("connector: expected ConnectAck, got %T", cAck.GetPayload())
	}
	if !connectAck.GetSuccess() {
		t.Fatalf("connector: ConnectAck not successful: %s", connectAck.GetMessage())
	}

	// --- Data relay: connector -> listener ---
	testPayload := []byte("hello from connector")

	if err := connectorSession.Send(&pb.Envelope{
		Payload: &pb.Envelope_Data{
			Data: &pb.Data{
				StreamId: 1,
				Payload:  testPayload,
			},
		},
	}); err != nil {
		t.Fatalf("connector: failed to send Data: %v", err)
	}

	// Listener should receive the relayed data.
	relayed, err := listenerSession.Receive()
	if err != nil {
		t.Fatalf("listener: failed to receive relayed Data: %v", err)
	}
	dataMsg := relayed.GetData()
	if dataMsg == nil {
		t.Fatalf("listener: expected Data message, got %T", relayed.GetPayload())
	}
	if dataMsg.GetStreamId() != 1 {
		t.Fatalf("listener: expected stream_id=1, got %d", dataMsg.GetStreamId())
	}
	if !bytes.Equal(dataMsg.GetPayload(), testPayload) {
		t.Fatalf("listener: payload mismatch: got %q, want %q", dataMsg.GetPayload(), testPayload)
	}

	// --- Data relay: listener -> connector ---
	replyPayload := []byte("hello from listener")

	if err := listenerSession.Send(&pb.Envelope{
		Payload: &pb.Envelope_Data{
			Data: &pb.Data{
				StreamId: 1,
				Payload:  replyPayload,
			},
		},
	}); err != nil {
		t.Fatalf("listener: failed to send reply Data: %v", err)
	}

	// Connector should receive the relayed reply.
	relayedReply, err := connectorSession.Receive()
	if err != nil {
		t.Fatalf("connector: failed to receive relayed reply: %v", err)
	}
	replyData := relayedReply.GetData()
	if replyData == nil {
		t.Fatalf("connector: expected Data message, got %T", relayedReply.GetPayload())
	}
	if !bytes.Equal(replyData.GetPayload(), replyPayload) {
		t.Fatalf("connector: reply payload mismatch: got %q, want %q", replyData.GetPayload(), replyPayload)
	}
}

func TestServer_PassphraseAuth_Correct(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithPassphrase("test-secret"))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	conn, session := dialAndHandshakeWithPassphrase(t, addr, "test-secret")
	defer conn.Close()

	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "auth-test-id",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Register: %v", err)
	}

	ack, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive RegisterAck: %v", err)
	}
	regAck := ack.GetRegisterAck()
	if regAck == nil {
		t.Fatalf("expected RegisterAck, got %T", ack.GetPayload())
	}
	if !regAck.GetSuccess() {
		t.Fatalf("RegisterAck not successful: %s", regAck.GetMessage())
	}
}

func TestServer_PassphraseAuth_Wrong(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithPassphrase("correct-password"))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	conn, session := dialAndHandshakeWithPassphrase(t, addr, "wrong-password")
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err := session.Receive()
	if err == nil {
		t.Fatal("expected error on Receive after wrong passphrase, got nil")
	}
}

func TestServer_PassphraseAuth_EmptyDefault(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "empty-passphrase-test",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Register: %v", err)
	}

	ack, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive RegisterAck: %v", err)
	}
	if !ack.GetRegisterAck().GetSuccess() {
		t.Fatalf("RegisterAck not successful: %s", ack.GetRegisterAck().GetMessage())
	}
}

// newTestLogger creates a Logger that writes to a temporary directory.
// Returns the logger and the log directory path.
func newTestLogger(t *testing.T) (*logging.Logger, string) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "logs")
	logger, err := logging.NewLogger(dir)
	if err != nil {
		t.Fatalf("failed to create test logger: %v", err)
	}
	return logger, dir
}

// readLogContent reads all log files in the directory and returns
// the concatenated content.
func readLogContent(t *testing.T, dir string) string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read log directory: %v", err)
	}

	var content strings.Builder
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("failed to read log file %s: %v", e.Name(), err)
		}
		content.Write(data)
	}
	return content.String()
}

func TestServer_Log_InvalidAuthPayload(t *testing.T) {
	logger, logDir := newTestLogger(t)

	srv := NewServer("127.0.0.1:0", WithLogger(logger))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()

	// Connect and perform handshake.
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to dial broker: %v", err)
	}
	defer conn.Close()

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		t.Fatalf("handshake failed: %v", err)
	}

	// Send a Register message instead of Authenticate (invalid payload).
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "not-an-auth-message",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send invalid payload: %v", err)
	}

	// Wait for the broker to process and close.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = session.Receive()

	// Allow broker to write the log entry.
	time.Sleep(100 * time.Millisecond)

	content := readLogContent(t, logDir)
	if !strings.Contains(content, "[AUTH]") {
		t.Errorf("expected AUTH tag in log, got: %q", content)
	}
	if !strings.Contains(content, "[WARN]") {
		t.Errorf("expected WARN severity in log, got: %q", content)
	}
	if !strings.Contains(content, "invalid auth payload from") {
		t.Errorf("expected 'invalid auth payload from' in log, got: %q", content)
	}
}

func TestServer_Log_PassphraseMismatch(t *testing.T) {
	logger, logDir := newTestLogger(t)

	srv := NewServer("127.0.0.1:0",
		WithPassphrase("correct-pass"),
		WithLogger(logger),
	)
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()

	// Perform handshake and send wrong passphrase.
	conn, session := dialAndHandshakeWithPassphrase(t, addr, "wrong-pass")
	defer conn.Close()

	// Wait for broker to close connection.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _ = session.Receive()

	time.Sleep(100 * time.Millisecond)

	content := readLogContent(t, logDir)
	if !strings.Contains(content, "[AUTH]") {
		t.Errorf("expected AUTH tag in log, got: %q", content)
	}
	if !strings.Contains(content, "passphrase mismatch from") {
		t.Errorf("expected 'passphrase mismatch from' in log, got: %q", content)
	}
	// Ensure the passphrase value is NOT logged.
	if strings.Contains(content, "wrong-pass") {
		t.Error("passphrase value should NOT be logged")
	}
}

func TestServer_Log_InvalidConnectionID(t *testing.T) {
	logger, logDir := newTestLogger(t)

	srv := NewServer("127.0.0.1:0", WithLogger(logger))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()

	// Connect as connector with a non-existent connection ID.
	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectRequest{
			ConnectRequest: &pb.ConnectRequest{
				ConnectionId: "non-existent-id-12345",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send ConnectRequest: %v", err)
	}

	// Read response (should be a failed ConnectAck).
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive response: %v", err)
	}

	connectAck := resp.GetConnectAck()
	if connectAck == nil {
		t.Fatalf("expected ConnectAck, got %T", resp.GetPayload())
	}
	if connectAck.GetSuccess() {
		t.Fatal("ConnectAck should not be successful for non-existent ID")
	}

	time.Sleep(100 * time.Millisecond)

	content := readLogContent(t, logDir)
	if !strings.Contains(content, "[ROUTING]") {
		t.Errorf("expected ROUTING tag in log, got: %q", content)
	}
	if !strings.Contains(content, "non-existent-id-12345") {
		t.Errorf("expected connection ID in log, got: %q", content)
	}
	if !strings.Contains(content, "invalid connection ID") {
		t.Errorf("expected 'invalid connection ID' in log, got: %q", content)
	}
}

// newTestIPFilter creates an IPsum DB loaded with the given IPs.
func newTestIPFilter(t *testing.T, ips map[string]int, minCount int) *ipsum.DB {
	t.Helper()

	db := ipsum.NewDB(minCount)

	var feed strings.Builder
	feed.WriteString("# test feed\n")
	for ip, count := range ips {
		feed.WriteString(ip)
		feed.WriteString("\t")
		feed.WriteString(strconv.Itoa(count))
		feed.WriteString("\n")
	}

	if err := db.LoadFromReader(strings.NewReader(feed.String()), nil); err != nil {
		t.Fatalf("failed to load test IP filter: %v", err)
	}
	return db
}

func TestServer_IPFilter_BlocksBlacklistedIP(t *testing.T) {
	logger, logDir := newTestLogger(t)

	// The test connects from 127.0.0.1, so block that.
	filter := newTestIPFilter(t, map[string]int{
		"127.0.0.1": 5,
	}, 3)

	srv := NewServer("127.0.0.1:0",
		WithLogger(logger),
		WithIPFilter(filter),
	)
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()

	// Try to connect — should be silently rejected.
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to dial broker: %v", err)
	}
	defer conn.Close()

	// The broker should close the connection immediately.
	// Try to read — should get EOF or error.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected connection to be closed by broker, but read succeeded")
	}

	// Allow broker to write the log entry.
	time.Sleep(100 * time.Millisecond)

	content := readLogContent(t, logDir)
	if !strings.Contains(content, "[IPBLOCK]") {
		t.Errorf("expected IPBLOCK tag in log, got: %q", content)
	}
	if !strings.Contains(content, "[WARN]") {
		t.Errorf("expected WARN severity in log, got: %q", content)
	}
	if !strings.Contains(content, "blocked connection from") {
		t.Errorf("expected 'blocked connection from' in log, got: %q", content)
	}
	if !strings.Contains(content, "ipsum count=5") {
		t.Errorf("expected 'ipsum count=5' in log, got: %q", content)
	}
}

func TestServer_IPFilter_AllowsNonBlacklistedIP(t *testing.T) {
	// Block a different IP, not 127.0.0.1.
	filter := newTestIPFilter(t, map[string]int{
		"10.20.30.40": 8,
	}, 3)

	srv := NewServer("127.0.0.1:0", WithIPFilter(filter))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()

	// Connect from 127.0.0.1 which is NOT blocked — handshake should work.
	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	// Send Register to confirm the connection works end-to-end.
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "ip-filter-allow-test",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Register: %v", err)
	}

	ack, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive RegisterAck: %v", err)
	}
	if !ack.GetRegisterAck().GetSuccess() {
		t.Fatalf("RegisterAck not successful: %s", ack.GetRegisterAck().GetMessage())
	}
}

func TestServer_IPFilter_NoFilterAllowsAll(t *testing.T) {
	// No IP filter set — all connections should be accepted.
	srv := NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()

	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "no-filter-test",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Register: %v", err)
	}

	ack, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive RegisterAck: %v", err)
	}
	if !ack.GetRegisterAck().GetSuccess() {
		t.Fatalf("RegisterAck not successful: %s", ack.GetRegisterAck().GetMessage())
	}
}
