# go-connect MVP Design

## Overview

A CLI tool in Go that creates encrypted TCP tunnels between two clients via a broker (Vermittler). The broker relays raw data between matched clients without seeing cleartext of the tunnel data beyond its own encrypted sessions.

## Scope (Minimal MVP)

- Broker server (`go-connect broker`)
- Listener client (`go-connect listen`)
- Connector client (`go-connect connect`)
- X25519 + HKDF key exchange per client-broker connection
- AES-256-GCM authenticated encryption
- Protobuf control messages with length-prefixed framing
- Stream multiplexing (multiple TCP connections through one tunnel)
- Heartbeat (15s interval, 45s timeout)
- Clean disconnect handling (notify other side when one client drops)

## Out of Scope (for MVP)

- Key rotation
- Access tokens for connection IDs
- Rate limiting
- Reconnect strategy
- Broker auth pinning / mTLS
- Prometheus metrics
- Half-close support

## Project Structure

```
go-connect/
├── main.go
├── go.mod / go.sum
├── proto/
│   └── messages.proto
├── types/
│   └── app_context.go
├── cmd/
│   ├── broker.go
│   ├── listen.go
│   ├── connect.go
│   └── version.go
├── pkg/
│   ├── crypto/
│   │   ├── handshake.go
│   │   └── aead.go
│   ├── protocol/
│   │   ├── framing.go
│   │   └── session.go
│   ├── broker/
│   │   ├── server.go
│   │   ├── client_conn.go
│   │   └── router.go
│   └── tunnel/
│       ├── listener.go
│       └── connector.go
└── pb/
    └── messages.pb.go
```

## Protocol Design

### Framing

Every frame on the wire:
```
[4 bytes: uint32 big-endian length][encrypted payload]
```

- Max frame size: 1 MB (protection against memory DoS)
- Payload is AES-256-GCM encrypted (except initial handshake)

### Protobuf Messages

```protobuf
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

### Handshake Flow

```
Client                          Broker
  |                               |
  |--- TCP Connect -------------->|
  |--- Handshake(pub_key_c) ----->|  (cleartext, public key only)
  |<-- Handshake(pub_key_b) ------|  (cleartext, public key only)
  |                               |
  |  [X25519 ECDH -> shared_secret]
  |  [HKDF(shared_secret) -> aes_key]
  |                               |
  |=== Encrypted from here on ====|
  |                               |
  |--- Register(conn_id) -------->|  (Listener)
  |<-- RegisterAck ---------------|
  |       OR                      |
  |--- ConnectRequest(conn_id) -->|  (Connector)
  |<-- ConnectAck ----------------|
```

The handshake messages are sent as length-prefixed protobuf but **not encrypted** (the key doesn't exist yet). Only the X25519 public keys are exchanged. From the next frame onwards, everything is AES-256-GCM encrypted.

### Stream Multiplexing

```
Client B (connect)         Broker              Client A (listen)
  |                          |                       |
  | [External TCP conn       |                       |
  |  on local port]          |                       |
  |-- OpenStream(id=1) ----->|-- OpenStream(id=1) -->|
  |                          |                 [Opens local TCP
  |                          |                  conn to target port]
  |<- OpenStreamAck(1) ------|<- OpenStreamAck ------|
  |                          |                       |
  |-- Data(id=1, bytes) ---->|-- Data(id=1) -------->|--- TCP write -->
  |<- Data(id=1, bytes) -----|<- Data(id=1) ---------|<-- TCP read ----
  |                          |                       |
  |-- CloseStream(id=1) ---->|-- CloseStream(id=1) ->|
```

- Stream IDs assigned by connector (uint32, monotonically increasing)
- Broker maps stream IDs 1:1 between listener and connector
- Multiple streams can be active simultaneously

## Crypto Details

- **Key Exchange:** `crypto/ecdh` with `ecdh.X25519()` (Go stdlib)
- **KDF:** `crypto/hkdf.Key(sha256.New, sharedSecret, nil, "go-connect-v1", 32)` - 32 bytes for AES-256
- **AEAD:** `crypto/aes` + `cipher.NewGCM()` - AES-256-GCM
- **Nonce:** Counter-based (12 bytes / uint96), separate counters for send and receive direction. Starts at 0, incremented per frame. No nonce reuse possible with < 2^96 frames.
- **All from Go standard library** (Go 1.26 has `crypto/hkdf` built-in)

## Heartbeat & Disconnect

- Heartbeat interval: 15 seconds
- Heartbeat timeout: 45 seconds (3 missed heartbeats)
- When a client disconnects (or times out):
  - Broker notifies the paired client via `Disconnect` message
  - Broker closes all streams for that connection
  - Broker removes the connection-ID mapping

## CLI Commands

```
go-connect broker <address>
  Starts the broker on the given address (e.g., "0.0.0.0:1781")

go-connect listen <local-port> <broker-address>
  Registers as listener with a generated connection ID
  Prints the connection ID for the connector to use

go-connect connect <broker-address> <connection-id> <local-port>
  Connects to a listener via broker
  Opens a local TCP server on <local-port>
  Forwards connections through the tunnel

go-connect version
  Prints version information
```

## Dependencies

- `github.com/spf13/cobra` - CLI framework (already in use)
- `github.com/google/uuid` - UUID generation for connection IDs
- `google.golang.org/protobuf` - Protobuf runtime
- `protoc` + `protoc-gen-go` - Protobuf compiler (build-time)
- Go stdlib: `crypto/ecdh`, `crypto/hkdf`, `crypto/aes`, `crypto/cipher`, `crypto/rand`, `crypto/sha256`
