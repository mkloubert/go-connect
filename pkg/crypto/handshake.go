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
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

const (
	// HKDFInfo is the context info string used for HKDF key derivation.
	HKDFInfo = "go-connect-v1"
)

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

// DeriveSharedKey performs an X25519 ECDH key exchange using the local key pair
// and the remote peer's public key bytes, then derives a 32-byte AES-256 key
// using HKDF-SHA256.
func DeriveSharedKey(local *KeyPair, remotePublicKeyBytes []byte) ([]byte, error) {
	remotePubKey, err := ecdh.X25519().NewPublicKey(remotePublicKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote public key: %w", err)
	}

	sharedSecret, err := local.PrivateKey.ECDH(remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("ECDH key exchange failed: %w", err)
	}

	derivedKey, err := hkdf.Key(sha256.New, sharedSecret, nil, HKDFInfo, AESKeySize)
	if err != nil {
		return nil, fmt.Errorf("HKDF key derivation failed: %w", err)
	}

	return derivedKey, nil
}
