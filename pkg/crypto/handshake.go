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

package crypto

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

const (
	// HKDFInfo is the context info string used for HKDF key derivation.
	HKDFInfo = "go-connect-v1"

	// HKDFInfoClientToServer is the HKDF info string for the client-to-server
	// direction key. The "client" is defined as the peer with the
	// lexicographically smaller public key.
	HKDFInfoClientToServer = "go-connect-v1-client-to-server"

	// HKDFInfoServerToClient is the HKDF info string for the server-to-client
	// direction key. The "server" is defined as the peer with the
	// lexicographically larger public key.
	HKDFInfoServerToClient = "go-connect-v1-server-to-client"
)

// DirectionalKeys holds two separate AES-256 keys for bidirectional
// encrypted communication, one for each direction. This prevents
// AES-GCM nonce reuse across directions.
type DirectionalKeys struct {
	SendKey []byte // Key for encrypting outgoing messages
	RecvKey []byte // Key for decrypting incoming messages
}

// KeyPair holds an X25519 private and public key pair.
type KeyPair struct {
	PrivateKey *ecdh.PrivateKey
	PublicKey  *ecdh.PublicKey
}

// GenerateKeyPair generates a new X25519 key pair for key exchange.
func GenerateKeyPair() (*KeyPair, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate X25519 key pair: %w", err)
	}

	return &KeyPair{
		PrivateKey: privateKey,
		PublicKey:  privateKey.PublicKey(),
	}, nil
}

// PublicKeyBytes returns the raw 32-byte public key.
func (kp *KeyPair) PublicKeyBytes() []byte {
	return kp.PublicKey.Bytes()
}

// DeriveDirectionalKeys performs an X25519 ECDH key exchange using the local
// key pair and the remote peer's public key bytes, then derives two separate
// 32-byte AES-256 keys using HKDF-SHA256 — one for each communication
// direction. This prevents AES-GCM nonce reuse: each direction uses its own
// key, so identical counter values on both sides never encrypt under the same
// key.
//
// Direction is determined by lexicographic comparison of the public keys:
// the peer with the smaller public key is the "client" and the other is the
// "server". Both sides derive the same two keys and independently assign
// send/recv based on their role.
func DeriveDirectionalKeys(local *KeyPair, remotePublicKeyBytes []byte) (*DirectionalKeys, error) {
	remotePubKey, err := ecdh.X25519().NewPublicKey(remotePublicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote public key: %w", err)
	}

	sharedSecret, err := local.PrivateKey.ECDH(remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH key exchange failed: %w", err)
	}

	// Build a per-session HKDF salt from both public keys in canonical
	// (lexicographic) order. Both sides independently compute the same
	// salt since they both hold both public keys. Hashing the
	// concatenation produces a fixed-size, unambiguous salt that provides
	// stronger key independence than the nil salt default (RFC 5869 §3.1).
	localPubBytes := local.PublicKeyBytes()
	var saltInput []byte
	if bytes.Compare(localPubBytes, remotePublicKeyBytes) < 0 {
		saltInput = append(localPubBytes, remotePublicKeyBytes...)
	} else {
		saltInput = append(remotePublicKeyBytes, localPubBytes...)
	}
	salt := sha256.Sum256(saltInput)

	// Derive two direction-specific keys using HKDF with different info strings.
	clientToServerKey, err := hkdf.Key(sha256.New, sharedSecret, salt[:], HKDFInfoClientToServer, AESKeySize)
	if err != nil {
		return nil, fmt.Errorf("HKDF key derivation (client-to-server) failed: %w", err)
	}

	serverToClientKey, err := hkdf.Key(sha256.New, sharedSecret, salt[:], HKDFInfoServerToClient, AESKeySize)
	if err != nil {
		return nil, fmt.Errorf("HKDF key derivation (server-to-client) failed: %w", err)
	}

	// Determine direction based on lexicographic ordering of public keys.
	// The peer with the smaller public key is the "client".
	if bytes.Compare(localPubBytes, remotePublicKeyBytes) < 0 {
		// Local is "client" (smaller public key)
		return &DirectionalKeys{
			SendKey: clientToServerKey,
			RecvKey: serverToClientKey,
		}, nil
	}

	// Local is "server" (larger public key)
	return &DirectionalKeys{
		SendKey: serverToClientKey,
		RecvKey: clientToServerKey,
	}, nil
}
