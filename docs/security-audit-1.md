# Security Audit & Code Review

This document contains a full analysis of the go-connect codebase, covering security vulnerabilities, architectural issues, stability risks, and code quality problems.

**Date:** 2026-02-28
**Scope:** All source files in the repository (MVP state, commit `5fbf9b9`)

---

## Table of Contents

- [Critical Issues](#critical-issues)
  - [1. AES-GCM Nonce Reuse](#1-aes-gcm-nonce-reuse)
  - [2. No Man-in-the-Middle Protection](#2-no-man-in-the-middle-protection)
  - [3. Broker Sees All Plaintext (No End-to-End Encryption)](#3-broker-sees-all-plaintext-no-end-to-end-encryption)
- [High Issues](#high-issues)
  - [4. Second Connector Overwrites First One](#4-second-connector-overwrites-first-one)
  - [5. No Timeout for OpenStreamAck](#5-no-timeout-for-openstreamack)
  - [6. Race Condition in handleOpenStream](#6-race-condition-in-handleopenstream)
- [Medium Issues](#medium-issues)
  - [7. No Dial Timeout](#7-no-dial-timeout)
  - [8. No Connection Limit / Rate Limiting on Broker](#8-no-connection-limit--rate-limiting-on-broker)
  - [9. HKDF Without Salt](#9-hkdf-without-salt)
  - [10. WriteFrame Is Not Atomic](#10-writeframe-is-not-atomic)
  - [11. Missing Connection ID Validation](#11-missing-connection-id-validation)
  - [12. Pending Streams Not Cleaned Up on Close](#12-pending-streams-not-cleaned-up-on-close)
  - [13. No Disconnect Signal From Client Side](#13-no-disconnect-signal-from-client-side)
  - [14. Error Messages Leak Internal Details](#14-error-messages-leak-internal-details)
- [Low Issues](#low-issues)
  - [15. No Unit Tests for pkg/tunnel](#15-no-unit-tests-for-pkgtunnel)
  - [16. Decrypt Does Not Validate Nonce Order](#16-decrypt-does-not-validate-nonce-order)
  - [17. select/default Busy-Loop Risk](#17-selectdefault-busy-loop-risk)
- [Summary](#summary)

---

## Critical Issues

### 1. AES-GCM Nonce Reuse

**Category:** Cryptography
**Files:** `pkg/crypto/aead.go`, `pkg/protocol/session.go:49-54`

Both sides of a connection (e.g. client and broker) derive the same shared key `K`. `NewSession` creates `sendEnc` and `recvEnc` with that same key, both counters starting at 0:

```
Client A sends message -> Encrypt(K, nonce=0)
Broker   sends message -> Encrypt(K, nonce=0)
```

The same `(key, nonce)` pair is used for **two different plaintexts**. With AES-GCM this is catastrophic: an attacker who can observe both ciphertexts can XOR them together to obtain the XOR of the plaintexts, breaking confidentiality.

**Recommended fix:** Derive two different keys, one per direction. For example, use HKDF with `"go-connect-v1-client-to-server"` and `"go-connect-v1-server-to-client"` as the info parameter.

---

### 2. No Man-in-the-Middle Protection

**Category:** Cryptography
**File:** `pkg/protocol/handshake.go`

The X25519 handshake exchanges public keys **without any authentication**. An attacker positioned between a client and the broker can:

1. Perform a separate handshake with each side
2. Establish two independent encrypted sessions
3. Read and modify all traffic in plaintext

There is no mechanism (certificates, pre-shared keys, TOFU, etc.) to verify the identity of the remote peer.

---

### 3. Broker Sees All Plaintext (No End-to-End Encryption)

**Category:** Architecture
**File:** `pkg/broker/server.go:211-239`

```go
env, err := client.Receive()  // Decrypts from Client A
// ...
peer.Send(env)                 // Re-encrypts for Client B
```

The broker decrypts every message from one client and re-encrypts it for the peer. The project description states: *"The broker only sends raw data back and forth"*. In reality, the broker has full access to all tunneled data (including VNC sessions, passwords, etc.).

For true end-to-end encryption, the clients would need to perform an additional key exchange **through the broker**, with the broker only relaying opaque bytes.

---

## High Issues

### 4. Second Connector Overwrites First One

**Category:** Logic / DoS
**File:** `pkg/broker/router.go:71-84`

```go
func (r *Router) LinkConnector(connectionID string, connector *ClientConn) error {
    // ...
    r.peers[listener] = connector  // Overwrites previous connector!
    r.peers[connector] = listener
    return nil
}
```

If a second connector connects with the same connection ID, the first connector is silently disconnected. Its peer link points to nothing. This is:

- A **DoS vector**: an attacker with the UUID can hijack existing tunnels
- A **data loss risk**: messages from the first connector are lost

---

### 5. No Timeout for OpenStreamAck

**Category:** Stability
**File:** `pkg/tunnel/connector.go:182-194`

```go
select {
case success := <-ackCh:
    // ...
case <-c.closeCh:
    // ...
}
// NO timeout case!
```

If the listener never responds (crash, network issue), the goroutine blocks **forever**. With many incoming connections this leads to goroutine leaks.

---

### 6. Race Condition in handleOpenStream

**Category:** Concurrency
**File:** `pkg/tunnel/listener.go:140-141`

```go
case env.GetOpenStream() != nil:
    streamID := env.GetOpenStream().GetStreamId()
    go l.handleOpenStream(streamID)  // Asynchronous!
```

The `handleOpenStream` goroutine must first call `net.Dial` and store the connection in the map. If `Data` messages for this stream ID arrive in the meantime, they are **silently dropped** (because `handleData` cannot find the stream in the map). The integration tests work around this with `time.Sleep(500ms)`.

---

## Medium Issues

### 7. No Dial Timeout

**Category:** Stability
**Files:** `pkg/tunnel/listener.go:70`, `pkg/tunnel/connector.go:71`, `pkg/tunnel/listener.go:171`

`net.Dial("tcp", ...)` has no timeout. If the broker or the local service is unreachable, the call can block for minutes (TCP default timeout).

**Recommended fix:** Use `net.DialTimeout` or context-based dialing.

---

### 8. No Connection Limit / Rate Limiting on Broker

**Category:** DoS
**File:** `pkg/broker/server.go:73-93`

The broker accepts unlimited connections. Each one requires a full X25519 handshake and AES-256-GCM setup. An attacker can:

- Open thousands of connections (resource exhaustion)
- Perform rapid stream creation/destruction
- Overload the server with handshakes

---

### 9. HKDF Without Salt

**Category:** Cryptography
**File:** `pkg/crypto/handshake.go:74`

```go
derivedKey, err := hkdf.Key(sha256.New, sharedSecret, nil, HKDFInfo, AESKeySize)
//                                                     ^^^
```

Using `nil` as the salt is allowed per RFC 5869 (internally becomes a chain of zeros), but a random salt would provide stronger key independence and is considered best practice.

---

### 10. WriteFrame Is Not Atomic

**Category:** Stability
**File:** `pkg/protocol/framing.go:40-58`

```go
func WriteFrame(w io.Writer, payload []byte) error {
    // ...
    w.Write(header)   // Write operation 1
    w.Write(payload)  // Write operation 2
}
```

Two separate `Write` calls. If an error occurs between them, the stream becomes corrupt (partial frame). A better approach would be to combine header and payload into a single buffer and write once, or use `net.Buffers` / `writev`.

---

### 11. Missing Connection ID Validation

**Category:** Input Validation
**File:** `pkg/broker/server.go:123`

The connection ID is accepted without any validation. An empty string, special characters, or extremely long strings are not rejected. The router stores them directly as map keys.

---

### 12. Pending Streams Not Cleaned Up on Close

**Category:** Memory
**File:** `pkg/tunnel/connector.go:190-194`

```go
case <-c.closeCh:
    localConn.Close()
    c.removeStream(streamID)
    return  // pendingStreams[streamID] is NOT removed!
```

Minor memory leak: `pendingStreams` map entries remain when the connector is closed while waiting for an ack.

---

### 13. No Disconnect Signal From Client Side

**Category:** Protocol
**Files:** `pkg/tunnel/listener.go:294-312`, `pkg/tunnel/connector.go:356-378`

When `Listener.Close()` or `Connector.Close()` is called, the TCP connection is simply closed, but **no `Disconnect` message** is sent to the broker. The broker only notices the disconnection on the next failed read. A clean protocol should send a Disconnect message first.

---

### 14. Error Messages Leak Internal Details

**Category:** Information Disclosure
**Files:** `pkg/broker/router.go:51`, `pkg/broker/router.go:77`

```go
return fmt.Errorf("connection ID %q is already registered", connectionID)
return fmt.Errorf("no listener registered for connection ID %q", connectionID)
```

These error messages are sent directly to the client (`server.go:150-157`). An attacker can use them to discover which connection IDs exist (enumeration).

---

## Low Issues

### 15. No Unit Tests for pkg/tunnel

**Category:** Test Coverage

The entire `pkg/tunnel/` package has no unit tests. Only the integration tests in `test/` cover it indirectly. Edge cases (concurrent stream operations, error handling, race conditions) are not tested.

---

### 16. Decrypt Does Not Validate Nonce Order

**Category:** Cryptography
**File:** `pkg/crypto/aead.go:99-118`

The `Decrypt` method increments the internal counter but uses the wire nonce. There is **no check** whether the received nonce matches the expected one. An attacker could:

- Replay messages (reuse old nonces)
- Reorder messages (change the sequence)

GCM protects the integrity of individual messages, but not the order or completeness of the stream.

---

### 17. select/default Busy-Loop Risk

**Category:** Stability
**Files:** `pkg/tunnel/listener.go:125-130`, `pkg/tunnel/connector.go:259-264`

```go
for {
    select {
    case <-l.closeCh:
        return
    default:
    }
    env, err := l.session.Receive()  // Blocks
```

This pattern is correct as long as `Receive()` blocks. But if `Receive()` fails immediately (e.g. on a closed connection), a short busy-loop can occur before the error terminates the loop. A context-based cancellation approach would be more robust.

---

## Summary

| Priority | Issue | Category |
|----------|-------|----------|
| CRITICAL | AES-GCM nonce reuse (same key, same nonce in both directions) | Cryptography |
| CRITICAL | No MITM protection (no authentication in handshake) | Cryptography |
| CRITICAL | Broker sees plaintext (no end-to-end encryption) | Architecture |
| HIGH | Second connector overwrites first one | Logic / DoS |
| HIGH | No timeout for OpenStreamAck | Stability |
| HIGH | Race condition in handleOpenStream (data loss) | Concurrency |
| MEDIUM | No dial timeout | Stability |
| MEDIUM | No connection limit on broker | DoS |
| MEDIUM | HKDF without salt | Cryptography |
| MEDIUM | WriteFrame is not atomic | Stability |
| MEDIUM | Missing connection ID validation | Input Validation |
| MEDIUM | Pending streams memory leak on close | Memory |
| MEDIUM | No disconnect signal from client side | Protocol |
| MEDIUM | Error messages leak internal details | Information Disclosure |
| LOW | No unit tests for pkg/tunnel | Test Coverage |
| LOW | No nonce order validation (replay/reorder) | Cryptography |
| LOW | select/default busy-loop risk | Stability |

The three critical cryptography issues (nonce reuse, no MITM protection, no E2E) must be resolved before any production use.
