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
	"testing"
)

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() returned error: %v", err)
	}
	if kp == nil {
		t.Fatal("GenerateKeyPair() returned nil")
	}
	if kp.PrivateKey == nil {
		t.Fatal("PrivateKey is nil")
	}
	if kp.PublicKey == nil {
		t.Fatal("PublicKey is nil")
	}

	pubBytes := kp.PublicKeyBytes()
	if len(pubBytes) != 32 {
		t.Fatalf("public key should be 32 bytes, got %d", len(pubBytes))
	}
}

func TestDeriveSharedKey(t *testing.T) {
	// Alice generates her key pair
	alice, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() for Alice returned error: %v", err)
	}

	// Bob generates his key pair
	bob, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() for Bob returned error: %v", err)
	}

	// Alice derives a shared key using Bob's public key
	aliceShared, err := DeriveSharedKey(alice, bob.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedKey() for Alice returned error: %v", err)
	}

	// Bob derives a shared key using Alice's public key
	bobShared, err := DeriveSharedKey(bob, alice.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedKey() for Bob returned error: %v", err)
	}

	// Both should derive the same shared key
	if !bytes.Equal(aliceShared, bobShared) {
		t.Fatal("Alice and Bob should derive the same shared key")
	}

	// The shared key should be 32 bytes (AES-256 key size)
	if len(aliceShared) != AESKeySize {
		t.Fatalf("shared key should be %d bytes, got %d", AESKeySize, len(aliceShared))
	}
}

func TestDeriveSharedKey_DifferentKeysProduceDifferentSecrets(t *testing.T) {
	// Alice generates her key pair
	alice, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() for Alice returned error: %v", err)
	}

	// Bob generates his key pair
	bob, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() for Bob returned error: %v", err)
	}

	// Charlie generates his key pair
	charlie, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() for Charlie returned error: %v", err)
	}

	// Alice-Bob shared key
	aliceBobShared, err := DeriveSharedKey(alice, bob.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedKey() Alice-Bob returned error: %v", err)
	}

	// Alice-Charlie shared key
	aliceCharlieShared, err := DeriveSharedKey(alice, charlie.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveSharedKey() Alice-Charlie returned error: %v", err)
	}

	// The shared keys should be different
	if bytes.Equal(aliceBobShared, aliceCharlieShared) {
		t.Fatal("shared keys with different peers should be different")
	}
}
