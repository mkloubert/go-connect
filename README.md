# go-connect

A CLI tool that creates encrypted TCP tunnels between two clients via a broker. The communication is secured using X25519 key exchange and AES-256-GCM authenticated encryption.

## How it works

```
Client A (listen)          Broker              Client B (connect)
     |                       |                       |
     |--- TCP + Handshake -->|                       |
     |--- Register(ID) ----->|                       |
     |                       |<-- TCP + Handshake ---|
     |                       |<-- Connect(ID) -------|
     |                       |                       |
     |<=== Encrypted Tunnel ==== Broker ==== Encrypted Tunnel ===>|
     |                       |                       |
Local Service (e.g. VNC)                        Local TCP Port
```

- **Client A** runs `listen` to expose a local service through the broker
- **Client B** runs `connect` to access that service through a local port
- The **broker** relays encrypted data between the two clients
- Neither client needs to know the other's IP address
- No firewall changes required -- both clients connect outbound to the broker

## Build

```bash
go build -o go-connect .
```

## Usage

### Start the broker

```bash
./go-connect broker 0.0.0.0:1781
```

### Expose a local service (Client A)

```bash
./go-connect listen 5900 1.2.3.4:1781
```

This prints a connection ID, e.g. `327ac625-3b0c-4bd7-ab1b-bb9d733774ae`.

### Connect to the service (Client B)

```bash
./go-connect connect 1.2.3.4:1781 327ac625-3b0c-4bd7-ab1b-bb9d733774ae 60000
```

Now connect to `localhost:60000` on Client B to access the service on Client A's port 5900.

### Example: VNC tunnel

```bash
# Client A: expose local VNC server
./go-connect listen 5900 broker.example.com:1781
# Output: Connection ID: 327ac625-3b0c-4bd7-ab1b-bb9d733774ae

# Client B: make VNC available locally
./go-connect connect broker.example.com:1781 327ac625-3b0c-4bd7-ab1b-bb9d733774ae 60000

# Now open a VNC viewer on Client B and connect to localhost:60000
```

### Version

```bash
./go-connect version
```

## Security

- **Key Exchange:** X25519 (Curve25519 ECDH) per client-broker connection
- **Key Derivation:** HKDF-SHA256 (RFC 5869)
- **Encryption:** AES-256-GCM authenticated encryption
- **Nonce Management:** Counter-based (no reuse possible)
- **Framing:** Length-prefixed with 1 MB max frame size (DoS protection)
- **Heartbeat:** 15s interval, 45s timeout for disconnect detection

## Architecture

```
go-connect/
├── main.go                    # Entry point
├── cmd/                       # CLI commands (broker, listen, connect, version)
├── pkg/
│   ├── crypto/                # X25519 handshake + AES-256-GCM encryption
│   ├── protocol/              # Framing, encrypted sessions, handshake protocol
│   ├── broker/                # Broker server, routing, client connections
│   └── tunnel/                # Listener and connector client logic
├── pb/                        # Generated protobuf code
├── proto/                     # Protobuf definitions
└── test/                      # Integration tests
```

## Testing

```bash
go test ./... -timeout 60s
```

## License

MIT License -- see [LICENSE](./LICENSE) for details.
