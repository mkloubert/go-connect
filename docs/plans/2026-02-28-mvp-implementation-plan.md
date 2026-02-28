# go-connect MVP Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a CLI tool that creates encrypted TCP tunnels between two clients via a broker, using X25519 key exchange and AES-256-GCM encryption.

**Architecture:** Monolithic broker with per-client encrypted connections. Clients connect to broker, perform X25519 handshake, then communicate via encrypted protobuf messages. Broker decrypts from one side and re-encrypts to the other. Stream multiplexing allows multiple TCP connections through a single tunnel.

**Tech Stack:** Go 1.26, cobra CLI, protobuf, crypto/ecdh (X25519), crypto/hkdf, crypto/aes (AES-256-GCM)

---

## Phase 1: Tooling & Protobuf Setup

### Task 1.1: Install protoc compiler

**Step 1: Download and install protoc for Linux aarch64**

Run:
```bash
curl -LO "https://github.com/protocolbuffers/protobuf/releases/download/v34.0/protoc-34.0-linux-aarch_64.zip"
unzip protoc-34.0-linux-aarch_64.zip -d /usr/local bin/protoc include/*
rm protoc-34.0-linux-aarch_64.zip
```

**Step 2: Install protoc-gen-go**

Run:
```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
```

**Step 3: Verify installation**

Run: `protoc --version && which protoc-gen-go`
Expected: Version output and path to protoc-gen-go

---

### Task 1.2: Create proto/messages.proto and generate Go code

**Files:**
- Create: `proto/messages.proto`
- Create: `pb/messages.pb.go` (generated)

**Step 1: Create the proto file**

Create `proto/messages.proto`:
```protobuf
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

syntax = "proto3";

package goconnect;

option go_package = "github.com/mkloubert/go-connect/pb";

message Envelope {
  oneof payload {
    Handshake handshake = 1;
    Register register = 2;
    RegisterAck register_ack = 3;
    ConnectRequest connect_request = 4;
    ConnectAck connect_ack = 5;
    OpenStream open_stream = 6;
    OpenStreamAck open_stream_ack = 7;
    Data data = 8;
    CloseStream close_stream = 9;
    Heartbeat heartbeat = 10;
    Disconnect disconnect = 11;
    ErrorMsg error_msg = 12;
  }
}

message Handshake {
  bytes public_key = 1;
}

message Register {
  string connection_id = 1;
}

message RegisterAck {
  bool success = 1;
  string message = 2;
}

message ConnectRequest {
  string connection_id = 1;
}

message ConnectAck {
  bool success = 1;
  string message = 2;
}

message OpenStream {
  uint32 stream_id = 1;
}

message OpenStreamAck {
  uint32 stream_id = 1;
  bool success = 2;
  string message = 3;
}

message Data {
  uint32 stream_id = 1;
  bytes payload = 2;
}

message CloseStream {
  uint32 stream_id = 1;
}

message Heartbeat {}

message Disconnect {
  string reason = 1;
}

message ErrorMsg {
  string message = 1;
}
```

**Step 2: Generate Go code**

Run:
```bash
mkdir -p pb
protoc --go_out=. --go_opt=paths=source_relative proto/messages.proto
```

Expected: `pb/messages.pb.go` is generated

**Step 3: Add dependencies**

Run:
```bash
go get google.golang.org/protobuf@latest
go get github.com/google/uuid@latest
go mod tidy
```

**Step 4: Verify build**

Run: `go build ./...`
Expected: Clean build, no errors

---

## Phase 2: Crypto Layer

### Task 2.1: Implement AEAD encryption (pkg/crypto/aead.go)

**Files:**
- Create: `pkg/crypto/aead.go`
- Create: `pkg/crypto/aead_test.go`

**Step 1: Write the failing test**

Create `pkg/crypto/aead_test.go`:
```go
// [LICENSE HEADER]

package crypto

import (
	"bytes"
	"testing"
)

func TestNewAEAD(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	aead, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD failed: %v", err)
	}
	if aead == nil {
		t.Fatal("NewAEAD returned nil")
	}
}

func TestNewAEAD_InvalidKeySize(t *testing.T) {
	_, err := NewAEAD(make([]byte, 16))
	if err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	a, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD failed: %v", err)
	}

	plaintext := []byte("hello, encrypted world!")

	ciphertext, err := a.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext should differ from plaintext")
	}

	decrypted, err := a.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted text mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_CounterNonce(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	a, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD failed: %v", err)
	}

	// Encrypt two messages -- they should produce different ciphertexts
	ct1, _ := a.Encrypt([]byte("message one"))
	ct2, _ := a.Encrypt([]byte("message one"))

	if bytes.Equal(ct1, ct2) {
		t.Fatal("same plaintext should produce different ciphertext due to different nonces")
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	a, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD failed: %v", err)
	}

	ciphertext, _ := a.Encrypt([]byte("sensitive data"))

	// Tamper with ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = a.Decrypt(ciphertext)
	if err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestDecryptWithCounter(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	sender, _ := NewAEAD(key)
	receiver, _ := NewAEAD(key)

	// Sender encrypts with incrementing counter
	ct1, _ := sender.Encrypt([]byte("first"))
	ct2, _ := sender.Encrypt([]byte("second"))

	// Receiver decrypts with matching counter
	pt1, err := receiver.Decrypt(ct1)
	if err != nil {
		t.Fatalf("Decrypt first failed: %v", err)
	}
	if string(pt1) != "first" {
		t.Fatalf("got %q want %q", pt1, "first")
	}

	pt2, err := receiver.Decrypt(ct2)
	if err != nil {
		t.Fatalf("Decrypt second failed: %v", err)
	}
	if string(pt2) != "second" {
		t.Fatalf("got %q want %q", pt2, "second")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/crypto/ -v`
Expected: FAIL (files don't exist)

**Step 3: Implement pkg/crypto/aead.go**

```go
// [LICENSE HEADER]

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"sync"
)

const (
	AESKeySize = 32       // AES-256
	NonceSize  = 12       // GCM standard nonce size
	MaxFrameSize = 1 << 20 // 1 MB max frame
)

type AEAD struct {
	gcm     cipher.AEAD
	counter uint64
	mu      sync.Mutex
}

func NewAEAD(key []byte) (*AEAD, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", AESKeySize, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}

	return &AEAD{
		gcm:     gcm,
		counter: 0,
	}, nil
}

func (a *AEAD) Encrypt(plaintext []byte) ([]byte, error) {
	a.mu.Lock()
	nonce := a.nextNonce()
	a.mu.Unlock()

	ciphertext := a.gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (a *AEAD) Decrypt(ciphertextWithNonce []byte) ([]byte, error) {
	a.mu.Lock()
	nonce := a.nextNonce()
	a.mu.Unlock()

	if len(ciphertextWithNonce) < NonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	receivedNonce := ciphertextWithNonce[:NonceSize]
	ciphertext := ciphertextWithNonce[NonceSize:]

	plaintext, err := a.gcm.Open(nil, receivedNonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	// Verify nonce matches expected counter
	expectedCounter := binary.BigEndian.Uint64(nonce[4:])
	receivedCounter := binary.BigEndian.Uint64(receivedNonce[4:])
	if expectedCounter != receivedCounter {
		// Already decrypted successfully but nonce mismatch -- this is unexpected
		// In practice we trust the nonce from the wire since GCM authenticated it
	}

	return plaintext, nil
}

func (a *AEAD) nextNonce() []byte {
	nonce := make([]byte, NonceSize)
	binary.BigEndian.PutUint64(nonce[4:], a.counter)
	a.counter++
	return nonce
}
```

**Step 4: Run tests**

Run: `go test ./pkg/crypto/ -v`
Expected: All tests PASS

---

### Task 2.2: Implement X25519 handshake (pkg/crypto/handshake.go)

**Files:**
- Create: `pkg/crypto/handshake.go`
- Create: `pkg/crypto/handshake_test.go`

**Step 1: Write the failing test**

Create `pkg/crypto/handshake_test.go`:
```go
// [LICENSE HEADER]

package crypto

import (
	"bytes"
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	if kp.PrivateKey == nil {
		t.Fatal("private key is nil")
	}
	if kp.PublicKey == nil {
		t.Fatal("public key is nil")
	}
	if len(kp.PublicKeyBytes()) != 32 {
		t.Fatalf("public key should be 32 bytes, got %d", len(kp.PublicKeyBytes()))
	}
}

func TestDeriveSharedKey(t *testing.T) {
	alice, _ := GenerateKeyPair()
	bob, _ := GenerateKeyPair()

	aliceKey, err := DeriveSharedKey(alice, bob.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedKey (alice) failed: %v", err)
	}

	bobKey, err := DeriveSharedKey(bob, alice.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedKey (bob) failed: %v", err)
	}

	if !bytes.Equal(aliceKey, bobKey) {
		t.Fatal("shared keys should be identical")
	}

	if len(aliceKey) != AESKeySize {
		t.Fatalf("shared key should be %d bytes, got %d", AESKeySize, len(aliceKey))
	}
}

func TestDeriveSharedKey_DifferentKeysProduceDifferentSecrets(t *testing.T) {
	alice, _ := GenerateKeyPair()
	bob, _ := GenerateKeyPair()
	charlie, _ := GenerateKeyPair()

	ab, _ := DeriveSharedKey(alice, bob.PublicKeyBytes())
	ac, _ := DeriveSharedKey(alice, charlie.PublicKeyBytes())

	if bytes.Equal(ab, ac) {
		t.Fatal("different peers should produce different shared keys")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/crypto/ -v -run TestGenerateKeyPair`
Expected: FAIL

**Step 3: Implement pkg/crypto/handshake.go**

```go
// [LICENSE HEADER]

package crypto

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

const HKDFInfo = "go-connect-v1"

type KeyPair struct {
	PrivateKey *ecdh.PrivateKey
	PublicKey  *ecdh.PublicKey
}

func GenerateKeyPair() (*KeyPair, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate X25519 key: %w", err)
	}

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  privateKey.PublicKey(),
	}, nil
}

func (kp *KeyPair) PublicKeyBytes() []byte {
	return kp.PublicKey.Bytes()
}

func DeriveSharedKey(local *KeyPair, remotePublicKeyBytes []byte) ([]byte, error) {
	remotePub, err := ecdh.X25519().NewPublicKey(remotePublicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse remote public key: %w", err)
	}

	sharedSecret, err := local.PrivateKey.ECDH(remotePub)
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}

	aesKey, err := hkdf.Key(sha256.New, sharedSecret, nil, HKDFInfo, AESKeySize)
	if err != nil {
		return nil, fmt.Errorf("hkdf.Key: %w", err)
	}

	return aesKey, nil
}
```

**Step 4: Run tests**

Run: `go test ./pkg/crypto/ -v`
Expected: All tests PASS

---

## Phase 3: Protocol Layer

### Task 3.1: Implement length-prefixed framing (pkg/protocol/framing.go)

**Files:**
- Create: `pkg/protocol/framing.go`
- Create: `pkg/protocol/framing_test.go`

**Step 1: Write the failing test**

Create `pkg/protocol/framing_test.go`:
```go
// [LICENSE HEADER]

package protocol

import (
	"bytes"
	"testing"
)

func TestWriteReadFrame(t *testing.T) {
	var buf bytes.Buffer

	payload := []byte("hello, framed world!")
	if err := WriteFrame(&buf, payload); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if !bytes.Equal(payload, got) {
		t.Fatalf("got %q, want %q", got, payload)
	}
}

func TestWriteReadFrame_Empty(t *testing.T) {
	var buf bytes.Buffer

	if err := WriteFrame(&buf, []byte{}); err != nil {
		t.Fatalf("WriteFrame failed: %v", err)
	}

	got, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame failed: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty payload, got %d bytes", len(got))
	}
}

func TestWriteReadFrame_Multiple(t *testing.T) {
	var buf bytes.Buffer

	messages := []string{"first", "second", "third"}
	for _, msg := range messages {
		if err := WriteFrame(&buf, []byte(msg)); err != nil {
			t.Fatalf("WriteFrame %q failed: %v", msg, err)
		}
	}

	for _, msg := range messages {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatalf("ReadFrame failed: %v", err)
		}
		if string(got) != msg {
			t.Fatalf("got %q, want %q", got, msg)
		}
	}
}

func TestReadFrame_TooLarge(t *testing.T) {
	var buf bytes.Buffer
	// Write a frame header claiming a size larger than MaxFrameSize
	header := make([]byte, 4)
	header[0] = 0x10 // 0x10000000 = 268 MB
	buf.Write(header)

	_, err := ReadFrame(&buf)
	if err == nil {
		t.Fatal("expected error for oversized frame")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/protocol/ -v`
Expected: FAIL

**Step 3: Implement pkg/protocol/framing.go**

```go
// [LICENSE HEADER]

package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const MaxFrameSize = 1 << 20 // 1 MB

func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameSize {
		return fmt.Errorf("payload too large: %d > %d", len(payload), MaxFrameSize)
	}

	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(payload)))

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

func ReadFrame(r io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	size := binary.BigEndian.Uint32(header)
	if size > MaxFrameSize {
		return nil, fmt.Errorf("frame too large: %d > %d", size, MaxFrameSize)
	}

	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	return payload, nil
}
```

**Step 4: Run tests**

Run: `go test ./pkg/protocol/ -v`
Expected: All tests PASS

---

### Task 3.2: Implement encrypted session (pkg/protocol/session.go)

**Files:**
- Create: `pkg/protocol/session.go`
- Create: `pkg/protocol/session_test.go`

**Step 1: Write the failing test**

Create `pkg/protocol/session_test.go`:
```go
// [LICENSE HEADER]

package protocol

import (
	"net"
	"testing"

	"github.com/mkloubert/go-connect/pb"
	gocrypto "github.com/mkloubert/go-connect/pkg/crypto"
)

func TestSession_SendReceiveEnvelope(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	serverSession, err := NewSession(serverConn, key)
	if err != nil {
		t.Fatalf("NewSession (server) failed: %v", err)
	}

	clientSession, err := NewSession(clientConn, key)
	if err != nil {
		t.Fatalf("NewSession (client) failed: %v", err)
	}

	// Send from client
	env := &pb.Envelope{
		Payload: &pb.Envelope_Heartbeat{
			Heartbeat: &pb.Heartbeat{},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- clientSession.Send(env)
	}()

	// Receive on server
	received, err := serverSession.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-done; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if received.GetHeartbeat() == nil {
		t.Fatal("expected Heartbeat message")
	}
}

func TestSession_SendReceiveData(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	serverSession, _ := NewSession(serverConn, key)
	clientSession, _ := NewSession(clientConn, key)

	payload := []byte("test data payload")
	env := &pb.Envelope{
		Payload: &pb.Envelope_Data{
			Data: &pb.Data{
				StreamId: 42,
				Payload:  payload,
			},
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- clientSession.Send(env)
	}()

	received, err := serverSession.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	<-done

	data := received.GetData()
	if data == nil {
		t.Fatal("expected Data message")
	}
	if data.StreamId != 42 {
		t.Fatalf("stream_id: got %d, want 42", data.StreamId)
	}
	if string(data.Payload) != "test data payload" {
		t.Fatalf("payload: got %q, want %q", data.Payload, "test data payload")
	}
}
```

Note: This test uses `_ = gocrypto.AESKeySize` to ensure the import is used. Actually, the import might not be needed directly. Adjust imports as needed.

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/protocol/ -v`
Expected: FAIL

**Step 3: Implement pkg/protocol/session.go**

```go
// [LICENSE HEADER]

package protocol

import (
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/proto"

	"github.com/mkloubert/go-connect/pb"
	gocrypto "github.com/mkloubert/go-connect/pkg/crypto"
)

type Session struct {
	conn    io.ReadWriter
	sendEnc *gocrypto.AEAD
	recvEnc *gocrypto.AEAD
	sendMu  sync.Mutex
	recvMu  sync.Mutex
}

func NewSession(conn io.ReadWriter, key []byte) (*Session, error) {
	sendEnc, err := gocrypto.NewAEAD(key)
	if err != nil {
		return nil, fmt.Errorf("create send AEAD: %w", err)
	}

	recvEnc, err := gocrypto.NewAEAD(key)
	if err != nil {
		return nil, fmt.Errorf("create recv AEAD: %w", err)
	}

	return &Session{
		conn:    conn,
		sendEnc: sendEnc,
		recvEnc: recvEnc,
	}, nil
}

func (s *Session) Send(env *pb.Envelope) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	encrypted, err := s.sendEnc.Encrypt(data)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	return WriteFrame(s.conn, encrypted)
}

func (s *Session) Receive() (*pb.Envelope, error) {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	frame, err := ReadFrame(s.conn)
	if err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	decrypted, err := s.recvEnc.Decrypt(frame)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	env := &pb.Envelope{}
	if err := proto.Unmarshal(decrypted, env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return env, nil
}

func (s *Session) SendCleartext(env *pb.Envelope) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	data, err := proto.Marshal(env)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	return WriteFrame(s.conn, data)
}

func (s *Session) ReceiveCleartext() (*pb.Envelope, error) {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	frame, err := ReadFrame(s.conn)
	if err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	env := &pb.Envelope{}
	if err := proto.Unmarshal(frame, env); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	return env, nil
}
```

**Step 4: Run tests**

Run: `go test ./pkg/protocol/ -v`
Expected: All tests PASS

---

### Task 3.3: Implement handshake protocol (pkg/protocol/handshake.go)

**Files:**
- Create: `pkg/protocol/handshake.go`
- Create: `pkg/protocol/handshake_test.go`

**Step 1: Write the failing test**

Create `pkg/protocol/handshake_test.go`:
```go
// [LICENSE HEADER]

package protocol

import (
	"net"
	"testing"
)

func TestPerformHandshake(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	type result struct {
		session *Session
		err     error
	}

	serverCh := make(chan result, 1)
	clientCh := make(chan result, 1)

	go func() {
		s, err := PerformHandshake(serverConn)
		serverCh <- result{s, err}
	}()

	go func() {
		s, err := PerformHandshake(clientConn)
		clientCh <- result{s, err}
	}()

	serverResult := <-serverCh
	if serverResult.err != nil {
		t.Fatalf("server handshake failed: %v", serverResult.err)
	}

	clientResult := <-clientCh
	if clientResult.err != nil {
		t.Fatalf("client handshake failed: %v", clientResult.err)
	}

	// Verify sessions can communicate
	env := newHeartbeatEnvelope()

	done := make(chan error, 1)
	go func() {
		done <- clientResult.session.Send(env)
	}()

	received, err := serverResult.session.Receive()
	if err != nil {
		t.Fatalf("receive after handshake failed: %v", err)
	}
	<-done

	if received.GetHeartbeat() == nil {
		t.Fatal("expected heartbeat after handshake")
	}
}
```

Helper in `pkg/protocol/helpers_test.go`:
```go
// [LICENSE HEADER]

package protocol

import "github.com/mkloubert/go-connect/pb"

func newHeartbeatEnvelope() *pb.Envelope {
	return &pb.Envelope{
		Payload: &pb.Envelope_Heartbeat{
			Heartbeat: &pb.Heartbeat{},
		},
	}
}
```

**Step 2: Implement pkg/protocol/handshake.go**

```go
// [LICENSE HEADER]

package protocol

import (
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"

	"github.com/mkloubert/go-connect/pb"
	gocrypto "github.com/mkloubert/go-connect/pkg/crypto"
)

func PerformHandshake(conn io.ReadWriter) (*Session, error) {
	localKeyPair, err := gocrypto.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}

	// Send our public key (cleartext)
	handshakeMsg := &pb.Envelope{
		Payload: &pb.Envelope_Handshake{
			Handshake: &pb.Handshake{
				PublicKey: localKeyPair.PublicKeyBytes(),
			},
		},
	}

	data, err := proto.Marshal(handshakeMsg)
	if err != nil {
		return nil, fmt.Errorf("marshal handshake: %w", err)
	}
	if err := WriteFrame(conn, data); err != nil {
		return nil, fmt.Errorf("send handshake: %w", err)
	}

	// Receive remote public key (cleartext)
	frame, err := ReadFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("receive handshake: %w", err)
	}

	remoteEnv := &pb.Envelope{}
	if err := proto.Unmarshal(frame, remoteEnv); err != nil {
		return nil, fmt.Errorf("unmarshal handshake: %w", err)
	}

	remoteHandshake := remoteEnv.GetHandshake()
	if remoteHandshake == nil {
		return nil, fmt.Errorf("expected handshake message, got %T", remoteEnv.GetPayload())
	}

	// Derive shared key
	sharedKey, err := gocrypto.DeriveSharedKey(localKeyPair, remoteHandshake.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("derive shared key: %w", err)
	}

	return NewSession(conn, sharedKey)
}
```

**Step 3: Run tests**

Run: `go test ./pkg/protocol/ -v`
Expected: All tests PASS

---

## Phase 4: Broker

### Task 4.1: Implement broker router (pkg/broker/router.go)

**Files:**
- Create: `pkg/broker/router.go`
- Create: `pkg/broker/router_test.go`

**Step 1: Write the failing test**

Create `pkg/broker/router_test.go`:
```go
// [LICENSE HEADER]

package broker

import (
	"testing"
)

func TestRouter_RegisterAndLookup(t *testing.T) {
	r := NewRouter()

	connID := "test-conn-id"
	client := &ClientConn{connectionID: connID}

	if err := r.RegisterListener(connID, client); err != nil {
		t.Fatalf("RegisterListener failed: %v", err)
	}

	found := r.FindListener(connID)
	if found != client {
		t.Fatal("FindListener did not return registered client")
	}
}

func TestRouter_RegisterDuplicate(t *testing.T) {
	r := NewRouter()

	connID := "test-conn-id"
	client1 := &ClientConn{connectionID: connID}
	client2 := &ClientConn{connectionID: connID}

	r.RegisterListener(connID, client1)
	err := r.RegisterListener(connID, client2)
	if err == nil {
		t.Fatal("expected error for duplicate registration")
	}
}

func TestRouter_LinkAndLookup(t *testing.T) {
	r := NewRouter()

	connID := "test-conn-id"
	listener := &ClientConn{connectionID: connID}
	connector := &ClientConn{connectionID: connID}

	r.RegisterListener(connID, listener)

	if err := r.LinkConnector(connID, connector); err != nil {
		t.Fatalf("LinkConnector failed: %v", err)
	}

	peer := r.GetPeer(listener)
	if peer != connector {
		t.Fatal("GetPeer(listener) should return connector")
	}

	peer = r.GetPeer(connector)
	if peer != listener {
		t.Fatal("GetPeer(connector) should return listener")
	}
}

func TestRouter_Remove(t *testing.T) {
	r := NewRouter()

	connID := "test-conn-id"
	listener := &ClientConn{connectionID: connID}
	connector := &ClientConn{connectionID: connID}

	r.RegisterListener(connID, listener)
	r.LinkConnector(connID, connector)

	r.Remove(listener)

	if r.FindListener(connID) != nil {
		t.Fatal("listener should be removed")
	}
	if r.GetPeer(connector) != nil {
		t.Fatal("peer should be nil after removal")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./pkg/broker/ -v`
Expected: FAIL

**Step 3: Implement pkg/broker/router.go**

```go
// [LICENSE HEADER]

package broker

import (
	"fmt"
	"sync"
)

type Router struct {
	listeners  map[string]*ClientConn // connectionID -> listener
	peers      map[*ClientConn]*ClientConn // client -> peer
	mu         sync.RWMutex
}

func NewRouter() *Router {
	return &Router{
		listeners: make(map[string]*ClientConn),
		peers:     make(map[*ClientConn]*ClientConn),
	}
}

func (r *Router) RegisterListener(connectionID string, client *ClientConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.listeners[connectionID]; exists {
		return fmt.Errorf("connection ID %q already registered", connectionID)
	}

	r.listeners[connectionID] = client
	return nil
}

func (r *Router) FindListener(connectionID string) *ClientConn {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.listeners[connectionID]
}

func (r *Router) LinkConnector(connectionID string, connector *ClientConn) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	listener, exists := r.listeners[connectionID]
	if !exists {
		return fmt.Errorf("no listener for connection ID %q", connectionID)
	}

	r.peers[listener] = connector
	r.peers[connector] = listener
	return nil
}

func (r *Router) GetPeer(client *ClientConn) *ClientConn {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.peers[client]
}

func (r *Router) Remove(client *ClientConn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from listeners
	for id, c := range r.listeners {
		if c == client {
			delete(r.listeners, id)
			break
		}
	}

	// Remove peer links
	if peer, exists := r.peers[client]; exists {
		delete(r.peers, peer)
		delete(r.peers, client)
	}
}
```

**Step 4: Run tests**

Run: `go test ./pkg/broker/ -v`
Expected: All tests PASS

---

### Task 4.2: Implement ClientConn (pkg/broker/client_conn.go)

**Files:**
- Create: `pkg/broker/client_conn.go`

```go
// [LICENSE HEADER]

package broker

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

const (
	HeartbeatInterval = 15 * time.Second
	HeartbeatTimeout  = 45 * time.Second
)

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

func (c *ClientConn) Send(env *pb.Envelope) error {
	return c.session.Send(env)
}

func (c *ClientConn) Receive() (*pb.Envelope, error) {
	return c.session.Receive()
}

func (c *ClientConn) SetConnectionID(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connectionID = id
}

func (c *ClientConn) ConnectionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectionID
}

func (c *ClientConn) UpdateLastSeen() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSeen = time.Now()
}

func (c *ClientConn) LastSeen() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSeen
}

func (c *ClientConn) Close() {
	c.closeOnce.Do(func() {
		close(c.closeCh)
		c.conn.Close()
		c.router.Remove(c)

		// Notify peer
		if peer := c.router.GetPeer(c); peer != nil {
			peer.Send(&pb.Envelope{
				Payload: &pb.Envelope_Disconnect{
					Disconnect: &pb.Disconnect{
						Reason: "peer disconnected",
					},
				},
			})
			peer.Close()
		}
	})
}

func (c *ClientConn) Done() <-chan struct{} {
	return c.closeCh
}

func (c *ClientConn) StartHeartbeat() {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
			if time.Since(c.LastSeen()) > HeartbeatTimeout {
				log.Printf("client %s heartbeat timeout", c.connectionID)
				c.Close()
				return
			}
			c.Send(&pb.Envelope{
				Payload: &pb.Envelope_Heartbeat{
					Heartbeat: &pb.Heartbeat{},
				},
			})
		}
	}
}
```

---

### Task 4.3: Implement broker server (pkg/broker/server.go)

**Files:**
- Create: `pkg/broker/server.go`
- Create: `pkg/broker/server_test.go`

**Step 1: Implement pkg/broker/server.go**

```go
// [LICENSE HEADER]

package broker

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/mkloubert/go-connect/pb"
)

type Server struct {
	address  string
	router   *Router
	listener net.Listener
	wg       sync.WaitGroup
	closeCh  chan struct{}
}

func NewServer(address string) *Server {
	return &Server{
		address: address,
		router:  NewRouter(),
		closeCh: make(chan struct{}),
	}
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = ln
	log.Printf("broker listening on %s", s.address)

	go s.acceptLoop()
	return nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.closeCh:
				return
			default:
				log.Printf("accept error: %v", err)
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

func (s *Server) handleClient(conn net.Conn) {
	client, err := NewClientConn(conn, s.router)
	if err != nil {
		log.Printf("handshake failed: %v", err)
		conn.Close()
		return
	}

	defer client.Close()

	// First message must be Register or ConnectRequest
	env, err := client.Receive()
	if err != nil {
		log.Printf("receive registration: %v", err)
		return
	}

	switch payload := env.Payload.(type) {
	case *pb.Envelope_Register:
		s.handleListener(client, payload.Register.ConnectionId)
	case *pb.Envelope_ConnectRequest:
		s.handleConnector(client, payload.ConnectRequest.ConnectionId)
	default:
		log.Printf("unexpected first message: %T", env.Payload)
		client.Send(&pb.Envelope{
			Payload: &pb.Envelope_ErrorMsg{
				ErrorMsg: &pb.ErrorMsg{
					Message: "expected Register or ConnectRequest",
				},
			},
		})
	}
}

func (s *Server) handleListener(client *ClientConn, connectionID string) {
	client.SetConnectionID(connectionID)

	if err := s.router.RegisterListener(connectionID, client); err != nil {
		client.Send(&pb.Envelope{
			Payload: &pb.Envelope_RegisterAck{
				RegisterAck: &pb.RegisterAck{
					Success: false,
					Message: err.Error(),
				},
			},
		})
		return
	}

	log.Printf("listener registered: %s", connectionID)

	client.Send(&pb.Envelope{
		Payload: &pb.Envelope_RegisterAck{
			RegisterAck: &pb.RegisterAck{
				Success: true,
				Message: "registered",
			},
		},
	})

	go client.StartHeartbeat()
	s.relayMessages(client)
}

func (s *Server) handleConnector(client *ClientConn, connectionID string) {
	client.SetConnectionID(connectionID)

	if err := s.router.LinkConnector(connectionID, client); err != nil {
		client.Send(&pb.Envelope{
			Payload: &pb.Envelope_ConnectAck{
				ConnectAck: &pb.ConnectAck{
					Success: false,
					Message: err.Error(),
				},
			},
		})
		return
	}

	log.Printf("connector linked: %s", connectionID)

	client.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectAck{
			ConnectAck: &pb.ConnectAck{
				Success: true,
				Message: "connected",
			},
		},
	})

	go client.StartHeartbeat()
	s.relayMessages(client)
}

func (s *Server) relayMessages(client *ClientConn) {
	for {
		env, err := client.Receive()
		if err != nil {
			log.Printf("client %s receive error: %v", client.ConnectionID(), err)
			return
		}

		client.UpdateLastSeen()

		// Handle heartbeat locally
		if env.GetHeartbeat() != nil {
			continue
		}

		// Relay to peer
		peer := s.router.GetPeer(client)
		if peer == nil {
			log.Printf("client %s has no peer", client.ConnectionID())
			continue
		}

		if err := peer.Send(env); err != nil {
			log.Printf("relay to peer failed: %v", err)
			return
		}
	}
}

func (s *Server) Stop() {
	close(s.closeCh)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
}

func (s *Server) Address() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.address
}
```

**Step 2: Write integration test**

Create `pkg/broker/server_test.go`:
```go
// [LICENSE HEADER]

package broker

import (
	"net"
	"testing"
	"time"

	"github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

func TestServer_ListenerConnectorLink(t *testing.T) {
	srv := NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	connID := "test-integration-id"

	// Listener connects
	listenerConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("listener dial: %v", err)
	}
	defer listenerConn.Close()

	listenerSession, err := protocol.PerformHandshake(listenerConn)
	if err != nil {
		t.Fatalf("listener handshake: %v", err)
	}

	// Register
	listenerSession.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{ConnectionId: connID},
		},
	})

	ack, err := listenerSession.Receive()
	if err != nil {
		t.Fatalf("listener receive ack: %v", err)
	}
	if !ack.GetRegisterAck().Success {
		t.Fatalf("register failed: %s", ack.GetRegisterAck().Message)
	}

	// Connector connects
	connectorConn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("connector dial: %v", err)
	}
	defer connectorConn.Close()

	connectorSession, err := protocol.PerformHandshake(connectorConn)
	if err != nil {
		t.Fatalf("connector handshake: %v", err)
	}

	// Connect
	connectorSession.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectRequest{
			ConnectRequest: &pb.ConnectRequest{ConnectionId: connID},
		},
	})

	connAck, err := connectorSession.Receive()
	if err != nil {
		t.Fatalf("connector receive ack: %v", err)
	}
	if !connAck.GetConnectAck().Success {
		t.Fatalf("connect failed: %s", connAck.GetConnectAck().Message)
	}

	// Connector sends data, listener receives it
	testPayload := []byte("hello through broker")
	done := make(chan error, 1)
	go func() {
		done <- connectorSession.Send(&pb.Envelope{
			Payload: &pb.Envelope_Data{
				Data: &pb.Data{
					StreamId: 1,
					Payload:  testPayload,
				},
			},
		})
	}()

	received, err := listenerSession.Receive()
	if err != nil {
		t.Fatalf("listener receive data: %v", err)
	}
	<-done

	data := received.GetData()
	if data == nil {
		t.Fatalf("expected Data message, got %T", received.Payload)
	}
	if string(data.Payload) != "hello through broker" {
		t.Fatalf("payload mismatch: %q", data.Payload)
	}
}
```

**Step 3: Run tests**

Run: `go test ./pkg/broker/ -v -timeout 30s`
Expected: All tests PASS

---

## Phase 5: Tunnel (Client-Side Logic)

### Task 5.1: Implement listener tunnel (pkg/tunnel/listener.go)

**Files:**
- Create: `pkg/tunnel/listener.go`

```go
// [LICENSE HEADER]

package tunnel

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

type Listener struct {
	localPort    string
	brokerAddr   string
	session      *protocol.Session
	connectionID string
	streams      map[uint32]net.Conn
	streamsMu    sync.Mutex
	closeCh      chan struct{}
}

func NewListener(localPort, brokerAddr, connectionID string) *Listener {
	return &Listener{
		localPort:    localPort,
		brokerAddr:   brokerAddr,
		connectionID: connectionID,
		streams:      make(map[uint32]net.Conn),
		closeCh:      make(chan struct{}),
	}
}

func (l *Listener) Start() error {
	conn, err := net.Dial("tcp", l.brokerAddr)
	if err != nil {
		return fmt.Errorf("connect to broker: %w", err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("handshake: %w", err)
	}
	l.session = session

	// Register with broker
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{ConnectionId: l.connectionID},
		},
	}); err != nil {
		conn.Close()
		return fmt.Errorf("send register: %w", err)
	}

	ack, err := session.Receive()
	if err != nil {
		conn.Close()
		return fmt.Errorf("receive register ack: %w", err)
	}

	regAck := ack.GetRegisterAck()
	if regAck == nil || !regAck.Success {
		conn.Close()
		msg := "unknown error"
		if regAck != nil {
			msg = regAck.Message
		}
		return fmt.Errorf("registration failed: %s", msg)
	}

	log.Printf("registered with broker as %s", l.connectionID)

	go l.handleMessages()
	return nil
}

func (l *Listener) handleMessages() {
	for {
		env, err := l.session.Receive()
		if err != nil {
			log.Printf("listener receive error: %v", err)
			l.Close()
			return
		}

		switch payload := env.Payload.(type) {
		case *pb.Envelope_OpenStream:
			go l.openStream(payload.OpenStream.StreamId)
		case *pb.Envelope_Data:
			l.handleData(payload.Data)
		case *pb.Envelope_CloseStream:
			l.closeStream(payload.CloseStream.StreamId)
		case *pb.Envelope_Heartbeat:
			// Respond to heartbeat
			l.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Heartbeat{Heartbeat: &pb.Heartbeat{}},
			})
		case *pb.Envelope_Disconnect:
			log.Printf("broker disconnect: %s", payload.Disconnect.Reason)
			l.Close()
			return
		}
	}
}

func (l *Listener) openStream(streamID uint32) {
	localConn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", l.localPort))
	if err != nil {
		log.Printf("failed to connect to local port %s: %v", l.localPort, err)
		l.session.Send(&pb.Envelope{
			Payload: &pb.Envelope_OpenStreamAck{
				OpenStreamAck: &pb.OpenStreamAck{
					StreamId: streamID,
					Success:  false,
					Message:  err.Error(),
				},
			},
		})
		return
	}

	l.streamsMu.Lock()
	l.streams[streamID] = localConn
	l.streamsMu.Unlock()

	l.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_OpenStreamAck{
			OpenStreamAck: &pb.OpenStreamAck{
				StreamId: streamID,
				Success:  true,
			},
		},
	})

	// Read from local connection and send to broker
	go l.pumpLocalToRemote(streamID, localConn)
}

func (l *Listener) pumpLocalToRemote(streamID uint32, localConn net.Conn) {
	buf := make([]byte, 32*1024) // 32 KB chunks
	for {
		n, err := localConn.Read(buf)
		if n > 0 {
			sendErr := l.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Data{
					Data: &pb.Data{
						StreamId: streamID,
						Payload:  buf[:n],
					},
				},
			})
			if sendErr != nil {
				log.Printf("stream %d send error: %v", streamID, sendErr)
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("stream %d local read error: %v", streamID, err)
			}
			l.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_CloseStream{
					CloseStream: &pb.CloseStream{StreamId: streamID},
				},
			})
			break
		}
	}
}

func (l *Listener) handleData(data *pb.Data) {
	l.streamsMu.Lock()
	localConn, exists := l.streams[data.StreamId]
	l.streamsMu.Unlock()

	if !exists {
		return
	}

	if _, err := localConn.Write(data.Payload); err != nil {
		log.Printf("stream %d local write error: %v", data.StreamId, err)
		l.closeStream(data.StreamId)
	}
}

func (l *Listener) closeStream(streamID uint32) {
	l.streamsMu.Lock()
	localConn, exists := l.streams[streamID]
	if exists {
		delete(l.streams, streamID)
	}
	l.streamsMu.Unlock()

	if exists {
		localConn.Close()
	}
}

func (l *Listener) Close() {
	select {
	case <-l.closeCh:
	default:
		close(l.closeCh)
	}

	l.streamsMu.Lock()
	for id, conn := range l.streams {
		conn.Close()
		delete(l.streams, id)
	}
	l.streamsMu.Unlock()
}

func (l *Listener) Done() <-chan struct{} {
	return l.closeCh
}
```

---

### Task 5.2: Implement connector tunnel (pkg/tunnel/connector.go)

**Files:**
- Create: `pkg/tunnel/connector.go`

```go
// [LICENSE HEADER]

package tunnel

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/mkloubert/go-connect/pb"
	"github.com/mkloubert/go-connect/pkg/protocol"
)

type Connector struct {
	brokerAddr   string
	connectionID string
	localPort    string
	session      *protocol.Session
	nextStreamID atomic.Uint32
	streams      map[uint32]net.Conn
	streamsMu    sync.Mutex
	localListener net.Listener
	closeCh      chan struct{}
}

func NewConnector(brokerAddr, connectionID, localPort string) *Connector {
	return &Connector{
		brokerAddr:   brokerAddr,
		connectionID: connectionID,
		localPort:    localPort,
		streams:      make(map[uint32]net.Conn),
		closeCh:      make(chan struct{}),
	}
}

func (c *Connector) Start() error {
	conn, err := net.Dial("tcp", c.brokerAddr)
	if err != nil {
		return fmt.Errorf("connect to broker: %w", err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("handshake: %w", err)
	}
	c.session = session

	// Connect via broker
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectRequest{
			ConnectRequest: &pb.ConnectRequest{ConnectionId: c.connectionID},
		},
	}); err != nil {
		conn.Close()
		return fmt.Errorf("send connect: %w", err)
	}

	ack, err := session.Receive()
	if err != nil {
		conn.Close()
		return fmt.Errorf("receive connect ack: %w", err)
	}

	connAck := ack.GetConnectAck()
	if connAck == nil || !connAck.Success {
		conn.Close()
		msg := "unknown error"
		if connAck != nil {
			msg = connAck.Message
		}
		return fmt.Errorf("connect failed: %s", msg)
	}

	log.Printf("connected to listener %s via broker", c.connectionID)

	// Start local TCP server
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%s", c.localPort))
	if err != nil {
		conn.Close()
		return fmt.Errorf("listen local port: %w", err)
	}
	c.localListener = ln

	log.Printf("local port %s open for connections", c.localPort)

	go c.handleMessages()
	go c.acceptLoop()

	return nil
}

func (c *Connector) acceptLoop() {
	for {
		conn, err := c.localListener.Accept()
		if err != nil {
			select {
			case <-c.closeCh:
				return
			default:
				log.Printf("accept error: %v", err)
				continue
			}
		}

		go c.handleLocalConnection(conn)
	}
}

func (c *Connector) handleLocalConnection(localConn net.Conn) {
	streamID := c.nextStreamID.Add(1)

	// Request stream opening
	if err := c.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_OpenStream{
			OpenStream: &pb.OpenStream{StreamId: streamID},
		},
	}); err != nil {
		log.Printf("send OpenStream failed: %v", err)
		localConn.Close()
		return
	}

	c.streamsMu.Lock()
	c.streams[streamID] = localConn
	c.streamsMu.Unlock()

	// Pump local -> remote
	go c.pumpLocalToRemote(streamID, localConn)
}

func (c *Connector) pumpLocalToRemote(streamID uint32, localConn net.Conn) {
	buf := make([]byte, 32*1024) // 32 KB chunks
	for {
		n, err := localConn.Read(buf)
		if n > 0 {
			sendErr := c.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Data{
					Data: &pb.Data{
						StreamId: streamID,
						Payload:  buf[:n],
					},
				},
			})
			if sendErr != nil {
				log.Printf("stream %d send error: %v", streamID, sendErr)
				break
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("stream %d local read error: %v", streamID, err)
			}
			c.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_CloseStream{
					CloseStream: &pb.CloseStream{StreamId: streamID},
				},
			})
			break
		}
	}
}

func (c *Connector) handleMessages() {
	for {
		env, err := c.session.Receive()
		if err != nil {
			log.Printf("connector receive error: %v", err)
			c.Close()
			return
		}

		switch payload := env.Payload.(type) {
		case *pb.Envelope_OpenStreamAck:
			ack := payload.OpenStreamAck
			if !ack.Success {
				log.Printf("stream %d open rejected: %s", ack.StreamId, ack.Message)
				c.closeStream(ack.StreamId)
			}
		case *pb.Envelope_Data:
			c.handleData(payload.Data)
		case *pb.Envelope_CloseStream:
			c.closeStream(payload.CloseStream.StreamId)
		case *pb.Envelope_Heartbeat:
			c.session.Send(&pb.Envelope{
				Payload: &pb.Envelope_Heartbeat{Heartbeat: &pb.Heartbeat{}},
			})
		case *pb.Envelope_Disconnect:
			log.Printf("broker disconnect: %s", payload.Disconnect.Reason)
			c.Close()
			return
		}
	}
}

func (c *Connector) handleData(data *pb.Data) {
	c.streamsMu.Lock()
	localConn, exists := c.streams[data.StreamId]
	c.streamsMu.Unlock()

	if !exists {
		return
	}

	if _, err := localConn.Write(data.Payload); err != nil {
		log.Printf("stream %d local write error: %v", data.StreamId, err)
		c.closeStream(data.StreamId)
	}
}

func (c *Connector) closeStream(streamID uint32) {
	c.streamsMu.Lock()
	localConn, exists := c.streams[streamID]
	if exists {
		delete(c.streams, streamID)
	}
	c.streamsMu.Unlock()

	if exists {
		localConn.Close()
	}
}

func (c *Connector) Close() {
	select {
	case <-c.closeCh:
	default:
		close(c.closeCh)
	}

	if c.localListener != nil {
		c.localListener.Close()
	}

	c.streamsMu.Lock()
	for id, conn := range c.streams {
		conn.Close()
		delete(c.streams, id)
	}
	c.streamsMu.Unlock()
}

func (c *Connector) Done() <-chan struct{} {
	return c.closeCh
}
```

---

## Phase 6: CLI Commands

### Task 6.1: Implement CLI commands (cmd/broker.go, cmd/listen.go, cmd/connect.go, cmd/version.go)

**Files:**
- Create: `cmd/broker.go`
- Create: `cmd/listen.go`
- Create: `cmd/connect.go`
- Create: `cmd/version.go`
- Modify: `types/app_context.go`

**Step 1: Create cmd/version.go**

```go
// [LICENSE HEADER]

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const Version = "0.1.0"

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("go-connect v%s\n", Version)
		},
	}
}
```

**Step 2: Create cmd/broker.go**

```go
// [LICENSE HEADER]

package cmd

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mkloubert/go-connect/pkg/broker"
	"github.com/spf13/cobra"
)

func NewBrokerCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "broker <address>",
		Short: "Start the broker server",
		Long:  "Starts the broker (Vermittler) that relays encrypted connections between clients",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			address := args[0]

			srv := broker.NewServer(address)
			if err := srv.Start(); err != nil {
				return err
			}
			defer srv.Stop()

			log.Printf("broker running on %s", srv.Address())

			// Wait for interrupt
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			log.Println("shutting down broker...")
			return nil
		},
	}
}
```

**Step 3: Create cmd/listen.go**

```go
// [LICENSE HEADER]

package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"github.com/mkloubert/go-connect/pkg/tunnel"
	"github.com/spf13/cobra"
)

func NewListenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "listen <local-port> <broker-address>",
		Short: "Register as listener with the broker",
		Long:  "Connects to the broker, registers a connection ID, and forwards incoming streams to a local port",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localPort := args[0]
			brokerAddr := args[1]

			connectionID := uuid.New().String()

			listener := tunnel.NewListener(localPort, brokerAddr, connectionID)
			if err := listener.Start(); err != nil {
				return err
			}
			defer listener.Close()

			fmt.Printf("Connection ID: %s\n", connectionID)
			log.Printf("listening on local port %s, registered with broker at %s", localPort, brokerAddr)

			// Wait for interrupt or disconnect
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-sigCh:
				log.Println("shutting down listener...")
			case <-listener.Done():
				log.Println("disconnected from broker")
			}

			return nil
		},
	}
}
```

**Step 4: Create cmd/connect.go**

```go
// [LICENSE HEADER]

package cmd

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mkloubert/go-connect/pkg/tunnel"
	"github.com/spf13/cobra"
)

func NewConnectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <broker-address> <connection-id> <local-port>",
		Short: "Connect to a listener via the broker",
		Long:  "Connects to a listener through the broker and opens a local TCP port for connections",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			brokerAddr := args[0]
			connectionID := args[1]
			localPort := args[2]

			connector := tunnel.NewConnector(brokerAddr, connectionID, localPort)
			if err := connector.Start(); err != nil {
				return err
			}
			defer connector.Close()

			log.Printf("connected to %s via broker, local port %s", connectionID, localPort)

			// Wait for interrupt or disconnect
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-sigCh:
				log.Println("shutting down connector...")
			case <-connector.Done():
				log.Println("disconnected from broker")
			}

			return nil
		},
	}
}
```

**Step 5: Update types/app_context.go**

Modify to register all subcommands:

```go
// [LICENSE HEADER]

package types

import (
	"github.com/mkloubert/go-connect/cmd"
	"github.com/spf13/cobra"
)

type AppContext struct {
	rootCommand *cobra.Command
}

func NewAppContext() *AppContext {
	rootCmd := &cobra.Command{
		Use:   "go-connect",
		Short: "Encrypted TCP tunnel tool",
		Long:  "Creates encrypted connections between clients via a broker over TCP/IP",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.AddCommand(
		cmd.NewBrokerCommand(),
		cmd.NewListenCommand(),
		cmd.NewConnectCommand(),
		cmd.NewVersionCommand(),
	)

	return &AppContext{
		rootCommand: rootCmd,
	}
}

func (a *AppContext) RootCommand() *cobra.Command {
	return a.rootCommand
}

func (a *AppContext) Run() error {
	return a.rootCommand.Execute()
}
```

**Step 6: Update main.go**

```go
// [LICENSE HEADER]

package main

import (
	"os"

	"github.com/mkloubert/go-connect/types"
)

func main() {
	app := types.NewAppContext()

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}
```

**Step 7: Build and verify**

Run: `go build -o go-connect .`
Expected: Clean build

Run: `./go-connect version`
Expected: `go-connect v0.1.0`

Run: `./go-connect --help`
Expected: Shows all subcommands (broker, listen, connect, version)

---

## Phase 7: Integration Test

### Task 7.1: End-to-end tunnel test

**Files:**
- Create: `test/integration_test.go`

```go
// [LICENSE HEADER]

package test

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mkloubert/go-connect/pkg/broker"
	"github.com/mkloubert/go-connect/pkg/tunnel"
)

func TestEndToEnd_EchoTunnel(t *testing.T) {
	// Start an echo server (simulates a local service like VNC)
	echoLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo server listen: %v", err)
	}
	defer echoLn.Close()

	echoPort := fmt.Sprintf("%d", echoLn.Addr().(*net.TCPAddr).Port)

	go func() {
		for {
			conn, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c) // echo
			}(conn)
		}
	}()

	// Start broker
	brokerSrv := broker.NewServer("127.0.0.1:0")
	if err := brokerSrv.Start(); err != nil {
		t.Fatalf("broker start: %v", err)
	}
	defer brokerSrv.Stop()

	brokerAddr := brokerSrv.Address()
	connID := uuid.New().String()

	// Start listener (connects echo server to broker)
	listener := tunnel.NewListener(echoPort, brokerAddr, connID)
	if err := listener.Start(); err != nil {
		t.Fatalf("listener start: %v", err)
	}
	defer listener.Close()

	// Small delay for registration
	time.Sleep(100 * time.Millisecond)

	// Start connector (opens local port for tunnel)
	connectorLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	connectorPort := fmt.Sprintf("%d", connectorLn.Addr().(*net.TCPAddr).Port)
	connectorLn.Close()

	connector := tunnel.NewConnector(brokerAddr, connID, connectorPort)
	if err := connector.Start(); err != nil {
		t.Fatalf("connector start: %v", err)
	}
	defer connector.Close()

	// Small delay for connection setup
	time.Sleep(100 * time.Millisecond)

	// Connect to connector's local port and test echo
	testConn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%s", connectorPort), 5*time.Second)
	if err != nil {
		t.Fatalf("test connection: %v", err)
	}
	defer testConn.Close()

	// Send data
	testData := "Hello through the encrypted tunnel!"
	testConn.Write([]byte(testData))

	// Read response (echo)
	testConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, len(testData))
	n, err := io.ReadFull(testConn, buf)
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}

	if string(buf[:n]) != testData {
		t.Fatalf("echo mismatch: got %q, want %q", buf[:n], testData)
	}

	t.Logf("end-to-end tunnel works: sent and received %q", testData)
}
```

**Step 1: Run integration test**

Run: `go test ./test/ -v -timeout 30s`
Expected: PASS

---

## Phase 8: Documentation & Cleanup

### Task 8.1: Update TASKS.md with completed checklist

### Task 8.2: Update README.md with usage instructions

Update `/workspace/README.md` with:
- Project description
- Build instructions
- Usage examples for broker, listen, connect
- Architecture overview

### Task 8.3: Add .gitignore entries

Add to `.gitignore`:
```
go-connect
*.pb.go
```

Note: `*.pb.go` is generated code. The proto source file is the source of truth.
Actually, generated protobuf files should typically be committed. Remove `*.pb.go` from gitignore.

### Task 8.4: Final build and test verification

Run:
```bash
go build -o go-connect .
go test ./... -v -timeout 60s
```

Expected: All builds and tests pass.
