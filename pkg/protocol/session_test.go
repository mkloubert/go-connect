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
	"crypto/rand"
	"net"
	"testing"

	gocrypto "github.com/mkloubert/go-connect/pkg/crypto"

	pb "github.com/mkloubert/go-connect/pb"
)

func TestSession_SendReceiveEnvelope(t *testing.T) {
	// Generate a random AES-256 key.
	key := make([]byte, gocrypto.AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create a bidirectional pipe.
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession, err := NewSession(clientConn, key)
	if err != nil {
		t.Fatalf("failed to create client session: %v", err)
	}

	serverSession, err := NewSession(serverConn, key)
	if err != nil {
		t.Fatalf("failed to create server session: %v", err)
	}

	// Send a heartbeat from client to server.
	env := newHeartbeatEnvelope()

	errCh := make(chan error, 1)
	go func() {
		errCh <- clientSession.Send(env)
	}()

	received, err := serverSession.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if received.GetHeartbeat() == nil {
		t.Fatal("expected Heartbeat payload, got nil")
	}
}

func TestSession_SendReceiveData(t *testing.T) {
	// Generate a random AES-256 key.
	key := make([]byte, gocrypto.AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientSession, err := NewSession(clientConn, key)
	if err != nil {
		t.Fatalf("failed to create client session: %v", err)
	}

	serverSession, err := NewSession(serverConn, key)
	if err != nil {
		t.Fatalf("failed to create server session: %v", err)
	}

	// Send a Data message with stream_id and payload.
	streamID := uint32(42)
	payload := []byte("encrypted tunnel data")
	env := newDataEnvelope(streamID, payload)

	errCh := make(chan error, 1)
	go func() {
		errCh <- clientSession.Send(env)
	}()

	received, err := serverSession.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("Send failed: %v", err)
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

func newDataEnvelope(streamID uint32, payload []byte) *pb.Envelope {
	return &pb.Envelope{
		Payload: &pb.Envelope_Data{
			Data: &pb.Data{
				StreamId: streamID,
				Payload:  payload,
			},
		},
	}
}
