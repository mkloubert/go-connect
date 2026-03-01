# Passphrase Authentication Design

## Goal

Add optional passphrase authentication to the broker. After the encrypted handshake, clients must authenticate with a SHA-256 hash of the passphrase. On mismatch, the broker silently closes the connection (no error response) to prevent information leakage.

## Protocol Change

New protobuf message `Authenticate` added to the `Envelope` oneof:

```protobuf
message Authenticate {
  bytes passphrase_hash = 1;  // SHA-256(passphrase), 32 bytes
}
```

## Connection Flow

```
Client                              Broker
  |                                   |
  |--- TCP Connect ------------------>|
  |<-- X25519 Handshake ------------->|  (encrypted session established)
  |                                   |
  |--- Authenticate(SHA256(pw)) ----->|
  |                                   |-- constant-time compare
  |                                   |
  |  [Match]    -> proceed with Register / ConnectRequest
  |  [Mismatch] -> broker closes TCP silently (no response)
  |  [Timeout]  -> broker closes TCP silently (no response)
```

## Changes

### 1. Protobuf Schema (`proto/messages.proto`)

- Add `Authenticate` message with `bytes passphrase_hash = 1`
- Add `Authenticate authenticate = 13` to Envelope oneof

### 2. Broker (`pkg/broker/`)

- `Server` gets `passphraseHash []byte` field
- New `WithPassphrase(passphrase string) ServerOption` (functional option)
- `handleClient()`: after handshake, call `authenticateClient(client)` before dispatch
- `authenticateClient()`:
  - Set read deadline (10s timeout)
  - Receive first message
  - Validate it is an `Authenticate` message
  - Compare hash with `crypto/subtle.ConstantTimeCompare()`
  - On failure: close connection silently, return error
  - On success: clear read deadline, return nil

### 3. Listener (`pkg/tunnel/listener.go`)

- Add `passphrase string` field
- After handshake, before Register: send `Authenticate{SHA256(passphrase)}`

### 4. Connector (`pkg/tunnel/connector.go`)

- Add `passphrase string` field
- After handshake, before ConnectRequest: send `Authenticate{SHA256(passphrase)}`

### 5. CLI (`cmd/`)

All three commands (broker, listen, connect) get:
- Flag: `--passphrase ""`
- Env var fallback: `GO_CONNECT_PASSPHRASE`
- Default: `""` (empty string)

### 6. Tests

- Unit test: broker authentication with correct/wrong/empty passphrase
- Unit test: silent close on wrong passphrase (no error message sent)
- Integration test: full tunnel with passphrase

## Security

- **Constant-time comparison** via `crypto/subtle.ConstantTimeCompare()` prevents timing attacks
- **No error response** on auth failure prevents information leakage
- **Timeout** (10s) prevents connection holding attacks
- **SHA-256 hashing** avoids plaintext passphrase in protocol messages (defense-in-depth)
- **Empty passphrase** is valid: `SHA256("")` is sent and compared normally
