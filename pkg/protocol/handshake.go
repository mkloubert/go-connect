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
	"fmt"
	"io"

	gocrypto "github.com/mkloubert/go-connect/pkg/crypto"

	pb "github.com/mkloubert/go-connect/pb"
	"google.golang.org/protobuf/proto"
)

// PerformHandshake executes an X25519 key exchange over the given connection
// and returns an encrypted Session. Both sides must call PerformHandshake
// concurrently on opposite ends of the connection.
//
// The handshake protocol:
//  1. Generate an X25519 key pair
//  2. Send Handshake(public_key) as a cleartext frame (concurrently)
//  3. Read Handshake(public_key) as a cleartext frame
//  4. Derive a shared AES-256 key via ECDH + HKDF
//  5. Return a new encrypted Session using the shared key
//
// The send and receive are performed concurrently to avoid deadlocks
// on synchronous connections (e.g., net.Pipe).
func PerformHandshake(conn io.ReadWriter) (*Session, error) {
	// Step 1: Generate X25519 key pair.
	keyPair, err := gocrypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("handshake: failed to generate key pair: %w", err)
	}

	// Step 2: Prepare and send our public key as a cleartext Handshake message.
	// The send is launched in a goroutine to avoid deadlocks on synchronous
	// connections where both sides must write before either can read.
	localHandshake := &pb.Envelope{
		Payload: &pb.Envelope_Handshake{
			Handshake: &pb.Handshake{
				PublicKey: keyPair.PublicKeyBytes(),
			},
		},
	}

	localData, err := proto.Marshal(localHandshake)
	if err != nil {
		return nil, fmt.Errorf("handshake: failed to marshal handshake: %w", err)
	}

	sendErrCh := make(chan error, 1)
	go func() {
		sendErrCh <- WriteFrame(conn, localData)
	}()

	// Step 3: Read the remote peer's public key.
	remoteFrame, err := ReadFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("handshake: failed to read remote handshake: %w", err)
	}

	// Wait for send to complete.
	if sendErr := <-sendErrCh; sendErr != nil {
		return nil, fmt.Errorf("handshake: failed to send handshake: %w", sendErr)
	}

	remoteEnvelope := &pb.Envelope{}
	if err := proto.Unmarshal(remoteFrame, remoteEnvelope); err != nil {
		return nil, fmt.Errorf("handshake: failed to unmarshal remote handshake: %w", err)
	}

	remoteHandshake := remoteEnvelope.GetHandshake()
	if remoteHandshake == nil {
		return nil, fmt.Errorf("handshake: expected Handshake message, got %T", remoteEnvelope.GetPayload())
	}

	remotePubKey := remoteHandshake.GetPublicKey()
	if len(remotePubKey) == 0 {
		return nil, fmt.Errorf("handshake: remote public key is empty")
	}

	// Step 4: Derive shared AES-256 key.
	sharedKey, err := gocrypto.DeriveSharedKey(keyPair, remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("handshake: failed to derive shared key: %w", err)
	}

	// Step 5: Create encrypted session.
	session, err := NewSession(conn, sharedKey)
	if err != nil {
		return nil, fmt.Errorf("handshake: failed to create session: %w", err)
	}

	return session, nil
}
