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
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

const (
	// AESKeySize is the required key size for AES-256 in bytes.
	AESKeySize = 32

	// NonceSize is the size of the GCM nonce in bytes.
	NonceSize = 12

	// MaxFrameSize is the maximum allowed plaintext frame size (1 MiB).
	MaxFrameSize = 1 << 20
)

// AEAD provides thread-safe AES-256-GCM encryption and decryption
// with counter-based nonces.
type AEAD struct {
	gcm     cipher.AEAD
	counter uint64
	mu      sync.Mutex
}

// NewAEAD creates a new AEAD instance with the given 32-byte AES-256 key.
// Returns an error if the key length is not exactly 32 bytes.
func NewAEAD(key []byte) (*AEAD, error) {
	if len(key) != AESKeySize {
		return nil, fmt.Errorf("invalid key size: got %d bytes, want %d bytes", len(key), AESKeySize)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &AEAD{
		gcm:     gcm,
		counter: 0,
	}, nil
}

// Encrypt encrypts the given plaintext using AES-256-GCM with a counter-based nonce.
// The returned ciphertext has the nonce prepended (first 12 bytes).
// This method is thread-safe.
func (a *AEAD) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) > MaxFrameSize {
		return nil, fmt.Errorf("plaintext exceeds maximum frame size: %d > %d", len(plaintext), MaxFrameSize)
	}

	a.mu.Lock()
	nonce := a.nextNonce()
	a.mu.Unlock()

	ciphertext := a.gcm.Seal(nil, nonce, plaintext, nil)

	// Prepend nonce to ciphertext
	result := make([]byte, NonceSize+len(ciphertext))
	copy(result[:NonceSize], nonce)
	copy(result[NonceSize:], ciphertext)

	return result, nil
}

// Decrypt decrypts the given ciphertext (with nonce prepended in the first 12 bytes)
// using AES-256-GCM. The nonce from the wire is used for decryption as GCM
// authenticates it. This method is thread-safe.
func (a *AEAD) Decrypt(ciphertextWithNonce []byte) ([]byte, error) {
	if len(ciphertextWithNonce) < NonceSize {
		return nil, errors.New("ciphertext too short: missing nonce")
	}

	nonce := ciphertextWithNonce[:NonceSize]
	ciphertext := ciphertextWithNonce[NonceSize:]

	// Advance our own counter to stay in sync, but use the wire nonce
	// for actual decryption since GCM authenticates it.
	a.mu.Lock()
	a.counter++
	a.mu.Unlock()

	plaintext, err := a.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// nextNonce generates a 12-byte nonce with the counter value encoded
// in big-endian in bytes 4-11. The counter is incremented after each call.
// MUST be called with a.mu held.
func (a *AEAD) nextNonce() []byte {
	nonce := make([]byte, NonceSize)
	// Bytes 0-3 are zero, bytes 4-11 hold the counter in big-endian
	binary.BigEndian.PutUint64(nonce[4:], a.counter)
	a.counter++
	return nonce
}
