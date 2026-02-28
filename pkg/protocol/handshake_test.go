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

package protocol

import (
	"bytes"
	"net"
	"testing"

	pb "github.com/mkloubert/go-connect/pb"
)

func TestPerformHandshake(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	type result struct {
		session *Session
		err     error
	}

	clientCh := make(chan result, 1)
	serverCh := make(chan result, 1)

	// Both sides perform the handshake concurrently.
	go func() {
		s, err := PerformHandshake(clientConn)
		clientCh <- result{session: s, err: err}
	}()

	go func() {
		s, err := PerformHandshake(serverConn)
		serverCh <- result{session: s, err: err}
	}()

	clientResult := <-clientCh
	if clientResult.err != nil {
		t.Fatalf("client handshake failed: %v", clientResult.err)
	}

	serverResult := <-serverCh
	if serverResult.err != nil {
		t.Fatalf("server handshake failed: %v", serverResult.err)
	}

	clientSession := clientResult.session
	serverSession := serverResult.session

	// Verify that encrypted communication works after handshake.
	// Send a Heartbeat from client to server.
	heartbeat := newHeartbeatEnvelope()

	errCh := make(chan error, 1)
	go func() {
		errCh <- clientSession.Send(heartbeat)
	}()

	received, err := serverSession.Receive()
	if err != nil {
		t.Fatalf("server Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("client Send failed: %v", err)
	}

	if received.GetHeartbeat() == nil {
		t.Fatal("expected Heartbeat payload after handshake, got nil")
	}

	// Send a Data message from server to client.
	streamID := uint32(99)
	payload := []byte("post-handshake encrypted data")
	dataEnv := &pb.Envelope{
		Payload: &pb.Envelope_Data{
			Data: &pb.Data{
				StreamId: streamID,
				Payload:  payload,
			},
		},
	}

	go func() {
		errCh <- serverSession.Send(dataEnv)
	}()

	received, err = clientSession.Receive()
	if err != nil {
		t.Fatalf("client Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server Send failed: %v", err)
	}

	data := received.GetData()
	if data == nil {
		t.Fatal("expected Data payload, got nil")
	}

	if data.GetStreamId() != streamID {
		t.Fatalf("stream_id mismatch: got %d, want %d", data.GetStreamId(), streamID)
	}

	if !bytes.Equal(data.GetPayload(), payload) {
		t.Fatalf("payload mismatch: got %q, want %q", data.GetPayload(), payload)
	}
}
