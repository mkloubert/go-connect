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
	"sync"

	gocrypto "github.com/mkloubert/go-connect/pkg/crypto"

	pb "github.com/mkloubert/go-connect/pb"
	"google.golang.org/protobuf/proto"
)

// Session provides encrypted, framed communication over a connection.
// It uses separate AEAD instances for sending and receiving to maintain
// independent nonce counters. All operations are thread-safe.
type Session struct {
	conn    io.ReadWriter
	sendEnc *gocrypto.AEAD
	recvEnc *gocrypto.AEAD
	sendMu  sync.Mutex
	recvMu  sync.Mutex
}

// NewSession creates a new encrypted Session over the given connection
// using separate 32-byte AES-256 keys for sending and receiving. This
// ensures that each direction uses a different key, preventing AES-GCM
// nonce reuse even though both directions start their counters at zero.
func NewSession(conn io.ReadWriter, sendKey, recvKey []byte) (*Session, error) {
	sendEnc, err := gocrypto.NewAEAD(sendKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create send AEAD: %w", err)
	}

	recvEnc, err := gocrypto.NewAEAD(recvKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create receive AEAD: %w", err)
	}

	return &Session{
		conn:    conn,
		sendEnc: sendEnc,
		recvEnc: recvEnc,
	}, nil
}

// Send marshals the given protobuf Envelope, encrypts it, and writes
// it as a length-prefixed frame. This method is thread-safe.
func (s *Session) Send(env *pb.Envelope) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	encrypted, err := s.sendEnc.Encrypt(data)
	if err != nil {
		return fmt.Errorf("failed to encrypt envelope: %w", err)
	}

	if err := WriteFrame(s.conn, encrypted); err != nil {
		return fmt.Errorf("failed to write encrypted frame: %w", err)
	}

	return nil
}

// Receive reads a length-prefixed frame, decrypts it, and unmarshals
// the protobuf Envelope. This method is thread-safe.
func (s *Session) Receive() (*pb.Envelope, error) {
	_, env, err := s.ReceiveRaw()
	return env, err
}

// ReceiveRaw reads a length-prefixed frame, decrypts it, and unmarshals
// the protobuf Envelope. It returns the raw decrypted bytes alongside
// the parsed envelope. On decryption failure the encrypted frame bytes
// are returned; on unmarshal failure the decrypted plaintext is returned.
// This method is thread-safe.
func (s *Session) ReceiveRaw() ([]byte, *pb.Envelope, error) {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	frame, err := ReadFrame(s.conn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read encrypted frame: %w", err)
	}

	plaintext, err := s.recvEnc.Decrypt(frame)
	if err != nil {
		return frame, nil, fmt.Errorf("failed to decrypt frame: %w", err)
	}

	env := &pb.Envelope{}
	if err := proto.Unmarshal(plaintext, env); err != nil {
		return plaintext, nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	return plaintext, env, nil
}

// SendCleartext marshals the given protobuf Envelope and writes it as a
// length-prefixed frame without encryption. This is used during the
// handshake phase before encryption keys are established.
func (s *Session) SendCleartext(env *pb.Envelope) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("failed to marshal envelope: %w", err)
	}

	if err := WriteFrame(s.conn, data); err != nil {
		return fmt.Errorf("failed to write cleartext frame: %w", err)
	}

	return nil
}

// ReceiveCleartext reads a length-prefixed frame and unmarshals the
// protobuf Envelope without decryption. This is used during the
// handshake phase before encryption keys are established.
func (s *Session) ReceiveCleartext() (*pb.Envelope, error) {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	frame, err := ReadFrame(s.conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read cleartext frame: %w", err)
	}

	env := &pb.Envelope{}
	if err := proto.Unmarshal(frame, env); err != nil {
		return nil, fmt.Errorf("failed to unmarshal envelope: %w", err)
	}

	return env, nil
}
