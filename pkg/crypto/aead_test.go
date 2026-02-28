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
	"crypto/rand"
	"testing"
)

func TestNewAEAD(t *testing.T) {
	key := make([]byte, AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	aead, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() returned error: %v", err)
	}
	if aead == nil {
		t.Fatal("NewAEAD() returned nil")
	}
}

func TestNewAEAD_InvalidKeySize(t *testing.T) {
	// 16-byte key should be rejected (we require 32 for AES-256)
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	aead, err := NewAEAD(key)
	if err == nil {
		t.Fatal("NewAEAD() should have returned error for 16-byte key")
	}
	if aead != nil {
		t.Fatal("NewAEAD() should return nil on error")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	encryptor, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() for encryptor returned error: %v", err)
	}

	decryptor, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() for decryptor returned error: %v", err)
	}

	plaintext := []byte("Hello, go-connect!")

	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}

	// Ciphertext must be longer than plaintext (nonce + GCM tag)
	if len(ciphertext) <= len(plaintext) {
		t.Fatal("ciphertext should be longer than plaintext")
	}

	decrypted, err := decryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() returned error: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Fatalf("decrypted text does not match: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_CounterNonce(t *testing.T) {
	key := make([]byte, AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	aead, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() returned error: %v", err)
	}

	plaintext := []byte("same plaintext")

	ct1, err := aead.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("first Encrypt() returned error: %v", err)
	}

	ct2, err := aead.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("second Encrypt() returned error: %v", err)
	}

	// Same plaintext must produce different ciphertext due to different counter nonces
	if bytes.Equal(ct1, ct2) {
		t.Fatal("encrypting the same plaintext twice should produce different ciphertext")
	}

	// Verify the nonces are different (first 12 bytes)
	nonce1 := ct1[:NonceSize]
	nonce2 := ct2[:NonceSize]
	if bytes.Equal(nonce1, nonce2) {
		t.Fatal("nonces should be different for consecutive encryptions")
	}
}

func TestDecrypt_Tampered(t *testing.T) {
	key := make([]byte, AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	encryptor, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() for encryptor returned error: %v", err)
	}

	decryptor, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() for decryptor returned error: %v", err)
	}

	plaintext := []byte("sensitive data")

	ciphertext, err := encryptor.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() returned error: %v", err)
	}

	// Tamper with the ciphertext (flip a bit in the encrypted payload, not the nonce)
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[NonceSize+1] ^= 0xFF

	_, err = decryptor.Decrypt(tampered)
	if err == nil {
		t.Fatal("Decrypt() should have returned error for tampered ciphertext")
	}
}

func TestDecryptWithCounter(t *testing.T) {
	key := make([]byte, AESKeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}

	sender, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() for sender returned error: %v", err)
	}

	receiver, err := NewAEAD(key)
	if err != nil {
		t.Fatalf("NewAEAD() for receiver returned error: %v", err)
	}

	messages := []string{
		"first message",
		"second message",
		"third message",
	}

	for i, msg := range messages {
		plaintext := []byte(msg)

		ciphertext, err := sender.Encrypt(plaintext)
		if err != nil {
			t.Fatalf("Encrypt() message %d returned error: %v", i, err)
		}

		decrypted, err := receiver.Decrypt(ciphertext)
		if err != nil {
			t.Fatalf("Decrypt() message %d returned error: %v", i, err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Fatalf("message %d: decrypted text does not match: got %q, want %q", i, decrypted, plaintext)
		}
	}
}
