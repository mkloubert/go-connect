# go-connect

A CLI tool that creates encrypted TCP tunnels between two clients via a broker. The communication is secured using X25519 key exchange and AES-256-GCM authenticated encryption.

## How it works

```
Client A (listen)          Broker              Client B (connect)
     |                       |                       |
     |--- TCP + Handshake -->|                       |
     |--- Authenticate ----->|                       |
     |--- Register(ID) ----->|                       |
     |                       |<-- TCP + Handshake ---|
     |                       |<-- Authenticate ------|
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

## Installation

### Download a release

Download a pre-built binary from the [Releases](https://github.com/mkloubert/go-connect/releases) page.

Binaries are available for:

| OS           | Architectures                                                                                     |
| ------------ | ------------------------------------------------------------------------------------------------- |
| Linux        | amd64, arm64, armv7, 386, loong64, mips, mipsle, mips64, mips64le, ppc64, ppc64le, riscv64, s390x |
| macOS        | amd64, arm64 (Apple Silicon)                                                                      |
| Windows      | amd64, arm64, 386                                                                                 |
| FreeBSD      | amd64, arm64, 386, armv7                                                                          |
| OpenBSD      | amd64, arm64, 386, armv7                                                                          |
| NetBSD       | amd64, arm64, 386                                                                                 |
| DragonflyBSD | amd64                                                                                             |
| Solaris      | amd64                                                                                             |
| AIX          | ppc64                                                                                             |

Each release includes SHA-256 checksums in `checksums.txt`.

Example (Linux amd64):

```bash
# Download and extract
tar -xzf go-connect-linux-amd64.tar.gz

# Make executable and move to PATH
chmod +x go-connect
sudo mv go-connect /usr/local/bin/
```

### Build from source

```bash
go build -o go-connect .
```

## Usage

### Start the broker

```bash
./go-connect broker
```

This starts the broker on the default address `0.0.0.0:1781`.

Custom bind address:

```bash
./go-connect broker --bind-to="192.168.1.10:2000"
# or bind to a specific port on all interfaces:
./go-connect b --bind-to=":2000"
```

With optional passphrase protection:

```bash
./go-connect broker --passphrase "my-secret"
```

### Expose a local service (Client A)

```bash
./go-connect listen --port=5900
```

This connects to the default broker at `127.0.0.1:1781` and prints a connection ID, e.g. `327ac625-3b0c-4bd7-ab1b-bb9d733774ae`.

With a remote broker:

```bash
./go-connect listen --port=5900 --broker="1.2.3.4:1781"
# or using short flags:
./go-connect l -p 5900 -b "1.2.3.4:1781"
```

With passphrase and custom connection ID:

```bash
./go-connect listen --port=5900 --broker="1.2.3.4:1781" --id="my-custom-id" --passphrase "my-secret"
```

The `--broker` flag supports flexible address formats:

| Format         | Result           |
| -------------- | ---------------- |
| `1.2.3.4:1781` | `1.2.3.4:1781`   |
| `:1781`        | `127.0.0.1:1781` |
| `1.2.3.4`      | `1.2.3.4:1781`   |
| _(empty)_      | `127.0.0.1:1781` |

### Connect to the service (Client B)

```bash
./go-connect connect --id="327ac625-3b0c-4bd7-ab1b-bb9d733774ae"
```

This connects to the default broker at `127.0.0.1:1781` and exposes the service on the default local port `12345`.

With custom broker and port:

```bash
./go-connect connect --broker="1.2.3.4:1781" --id="327ac625-3b0c-4bd7-ab1b-bb9d733774ae" --port=60000
# or using short flags:
./go-connect c -b "1.2.3.4:1781" -i "327ac625-3b0c-4bd7-ab1b-bb9d733774ae" -p 60000
```

With passphrase:

```bash
./go-connect connect --broker="1.2.3.4:1781" --id="327ac625-3b0c-4bd7-ab1b-bb9d733774ae" --port=60000 --passphrase "my-secret"
```

Now connect to `localhost:60000` on Client B to access the service on Client A.

### Example: VNC tunnel

```bash
# Client A: expose local VNC server
./go-connect listen --port=5900 --broker="broker.example.com:1781" --passphrase "my-secret"
# Output: Connection ID: 327ac625-3b0c-4bd7-ab1b-bb9d733774ae

# Client B: make VNC available locally
./go-connect connect --broker="broker.example.com:1781" --id="327ac625-3b0c-4bd7-ab1b-bb9d733774ae" --port=60000 --passphrase "my-secret"

# Now open a VNC viewer on Client B and connect to localhost:60000
```

### Command reference

| Command   | Alias | Required flags | Optional flags                                 |
| --------- | ----- | -------------- | ---------------------------------------------- |
| `broker`  | `b`   |                | `--bind-to`, `--passphrase`                    |
| `listen`  | `l`   | `--port`/`-p`  | `--broker`/`-b`, `--id`/`-i`, `--passphrase`   |
| `connect` | `c`   | `--id`/`-i`    | `--broker`/`-b`, `--port`/`-p`, `--passphrase` |
| `version` |       |                |                                                |

### Global flags

These flags are available on all commands:

| Flag            | Short | Default | Description                                                                     |
| --------------- | ----- | ------- | ------------------------------------------------------------------------------- |
| `--verbose`     | `-v`  | `false` | Show technical details (handshake timing, stream IDs, debug info)               |
| `--quiet`       | `-q`  | `false` | Only errors and essential info (connection ID for listener)                     |
| `--no-color`    |       | `false` | Disable colored output (automatic when piped; also respects `NO_COLOR` env var) |
| `--max-retries` |       | `10`    | Max reconnect attempts (`-1` = infinite, `0` = disabled)                        |

### Output modes

**Normal mode** (default): Colored output with status messages, connection info, and hints on errors.

**Verbose mode** (`--verbose`): Adds debug information like handshake timing, heartbeat sequences, and stream IDs.

**Quiet mode** (`--quiet`): Minimal output. The `listen` command only prints the connection ID -- useful for scripting:

```bash
ID=$(./go-connect listen -p 5900 -q &)
```

### Auto-reconnect

By default, `listen` and `connect` automatically reconnect when the broker connection is lost.

Reconnect uses exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (capped), with ±25% jitter.

```bash
# Disable reconnect (exit on disconnect):
./go-connect listen -p 5900 --max-retries=0

# Infinite reconnect attempts:
./go-connect listen -p 5900 --max-retries=-1
```

Press `Ctrl+C` during reconnect to cancel immediately.

### Environment variables

All commands support environment variables as alternatives to flags:

| Variable                 | Commands                | Purpose                              |
| ------------------------ | ----------------------- | ------------------------------------ |
| `GO_CONNECT_PASSPHRASE`  | broker, listen, connect | Passphrase for authentication        |
| `GO_CONNECT_ID`          | listen, connect         | Connection ID                        |
| `GO_CONNECT_VERBOSE`     | all                     | Set to `1` to enable verbose mode    |
| `GO_CONNECT_QUIET`       | all                     | Set to `1` to enable quiet mode      |
| `GO_CONNECT_MAX_RETRIES` | listen, connect         | Max reconnect attempts               |
| `NO_COLOR`               | all                     | Set to `1` to disable colored output |

```bash
export GO_CONNECT_PASSPHRASE="my-secret"
export GO_CONNECT_ID="327ac625-3b0c-4bd7-ab1b-bb9d733774ae"
./go-connect broker
./go-connect listen --port=5900 --broker="1.2.3.4:1781"
./go-connect connect --broker="1.2.3.4:1781" --port=60000
```

### Version

```bash
./go-connect version
```

## Security

- **Key Exchange:** X25519 (Curve25519 ECDH) per client-broker connection
- **Key Derivation:** HKDF-SHA256 (RFC 5869)
- **Encryption:** AES-256-GCM authenticated encryption
- **Authentication:** Optional passphrase (SHA-256 hash, constant-time comparison)
- **Nonce Management:** Counter-based (no reuse possible)
- **Framing:** Length-prefixed with 1 MB max frame size (DoS protection)
- **Heartbeat:** 15s interval, 45s timeout for disconnect detection
- **Silent Rejection:** Wrong passphrase causes silent connection close (no information leakage)

## Architecture

```
go-connect/
├── main.go                    # Entry point
├── cmd/                       # CLI commands (broker, listen, connect, version)
├── pkg/
│   ├── crypto/                # X25519 handshake + AES-256-GCM encryption
│   ├── protocol/              # Framing, encrypted sessions, handshake protocol
│   ├── broker/                # Broker server, routing, client connections
│   ├── tunnel/                # Listener, connector, and reconnect logic
│   └── ui/                    # Colored terminal output and network interface listing
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
