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

func TestDeriveDirectionalKeys(t *testing.T) {
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

	// Alice derives directional keys using Bob's public key
	aliceKeys, err := DeriveDirectionalKeys(alice, bob.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveDirectionalKeys() for Alice returned error: %v", err)
	}

	// Bob derives directional keys using Alice's public key
	bobKeys, err := DeriveDirectionalKeys(bob, alice.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveDirectionalKeys() for Bob returned error: %v", err)
	}

	// Alice's send key must equal Bob's recv key (and vice versa)
	if !bytes.Equal(aliceKeys.SendKey, bobKeys.RecvKey) {
		t.Fatal("Alice's SendKey should equal Bob's RecvKey")
	}

	if !bytes.Equal(aliceKeys.RecvKey, bobKeys.SendKey) {
		t.Fatal("Alice's RecvKey should equal Bob's SendKey")
	}

	// The two directional keys must be different from each other
	if bytes.Equal(aliceKeys.SendKey, aliceKeys.RecvKey) {
		t.Fatal("SendKey and RecvKey should be different")
	}

	// All keys should be 32 bytes (AES-256 key size)
	if len(aliceKeys.SendKey) != AESKeySize {
		t.Fatalf("SendKey should be %d bytes, got %d", AESKeySize, len(aliceKeys.SendKey))
	}
	if len(aliceKeys.RecvKey) != AESKeySize {
		t.Fatalf("RecvKey should be %d bytes, got %d", AESKeySize, len(aliceKeys.RecvKey))
	}
}

func TestDeriveDirectionalKeys_DifferentPeersProduceDifferentKeys(t *testing.T) {
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

	// Alice-Bob keys
	aliceBobKeys, err := DeriveDirectionalKeys(alice, bob.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveDirectionalKeys() Alice-Bob returned error: %v", err)
	}

	// Alice-Charlie keys
	aliceCharlieKeys, err := DeriveDirectionalKeys(alice, charlie.PublicKeyBytes())
	if err != nil {
		t.Fatalf("DeriveDirectionalKeys() Alice-Charlie returned error: %v", err)
	}

	// Keys with different peers should be different
	if bytes.Equal(aliceBobKeys.SendKey, aliceCharlieKeys.SendKey) {
		t.Fatal("send keys with different peers should be different")
	}
	if bytes.Equal(aliceBobKeys.RecvKey, aliceCharlieKeys.RecvKey) {
		t.Fatal("recv keys with different peers should be different")
	}
}
