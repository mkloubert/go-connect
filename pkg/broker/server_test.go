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
	"net"
	"testing"
	"time"

	pb "github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

// dialAndHandshake connects to the broker at the given address, performs
// the handshake, and returns an encrypted session over the connection.
func dialAndHandshake(t *testing.T, addr string) (net.Conn, *protocol.Session) {
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
