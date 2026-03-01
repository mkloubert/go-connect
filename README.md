# go-connect

A CLI tool that creates encrypted TCP tunnels between two clients via a broker. The communication is secured using X25519 key exchange and AES-256-GCM authenticated encryption.

## How it works

```
Client A (listen)                Broker                     Client B (connect)
     |                             |                              |
     |------ TCP + Handshake ----->|                              |
     |------ Authenticate -------->|                              |
     |------ Register(ID) -------->|                              |
     |                             |<----- TCP + Handshake -------|
     |                             |<----- Authenticate ----------|
     |                             |<----- Connect(ID) -----------|
     |                             |                              |
     |<=== Encrypted Tunnel ==== Broker ==== Encrypted Tunnel ===>|
     |                             |                              |
Local Service (e.g. VNC)                                      Local TCP Port
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
| AIX          | ppc64                                                                                             |
| DragonflyBSD | amd64                                                                                             |
| FreeBSD      | amd64, arm64, 386, armv7                                                                          |
| Linux        | amd64, arm64, armv7, 386, loong64, mips, mipsle, mips64, mips64le, ppc64, ppc64le, riscv64, s390x |
| macOS        | amd64, arm64 (Apple Silicon)                                                                      |
| NetBSD       | 386, amd64, arm64                                                                                 |
| OpenBSD      | 386, amd64, arm64, armv7                                                                          |
| Solaris      | amd64                                                                                             |
| Windows      | 386, amd64, arm64                                                                                 |

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

| Command   | Alias | Required flags | Optional flags                                                                               |
| --------- | ----- | -------------- | -------------------------------------------------------------------------------------------- |
| `broker`  | `b`   |                | `--bind-to`, `--passphrase`, `--enable-ipsum`, `--enable-geo-blocker`, `--blocked-countries` |
| `connect` | `c`   | `--id`/`-i`    | `--broker`/`-b`, `--port`/`-p`, `--passphrase`                                               |
| `listen`  | `l`   | `--port`/`-p`  | `--broker`/`-b`, `--id`/`-i`, `--passphrase`                                                 |
| `version` |       |                |                                                                                              |

### Global flags

These flags are available on all commands:

| Flag            | Short | Default | Description                                                                     |
| --------------- | ----- | ------- | ------------------------------------------------------------------------------- |
| `--max-retries` |       | `10`    | Max reconnect attempts (`-1` = infinite, `0` = disabled)                        |
| `--no-color`    |       | `false` | Disable colored output (automatic when piped; also respects `NO_COLOR` env var) |
| `--quiet`       | `-q`  | `false` | Only errors and essential info (connection ID for listener)                     |
| `--verbose`     | `-v`  | `false` | Show technical details (handshake timing, stream IDs, debug info)               |

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

| Variable                        | Commands                | Purpose                                                                    |
| ------------------------------- | ----------------------- | -------------------------------------------------------------------------- |
| `GO_CONNECT_BLOCKED_COUNTRIES`  | broker                  | Comma-separated ISO country codes to block                                 |
| `GO_CONNECT_ENABLE_GEO_BLOCKER` | broker                  | Set to `1` to enable GeoLite2 country blocking                             |
| `GO_CONNECT_ENABLE_IPSUM`       | broker                  | Set to `1` to enable IPsum IP blocking                                     |
| `GO_CONNECT_GEO_DB`             | broker                  | Path to GeoLite2 mmdb file (default: `GeoLite2.mmdb` in working directory) |
| `GO_CONNECT_ID`                 | listen, connect         | Connection ID                                                              |
| `GO_CONNECT_IPSUM_SOURCE`       | broker                  | Custom URL for the IPsum feed                                              |
| `GO_CONNECT_MAX_RETRIES`        | listen, connect         | Max reconnect attempts                                                     |
| `GO_CONNECT_PASSPHRASE`         | broker, listen, connect | Passphrase for authentication                                              |
| `GO_CONNECT_QUIET`              | all                     | Set to `1` to enable quiet mode                                            |
| `GO_CONNECT_VERBOSE`            | all                     | Set to `1` to enable verbose mode                                          |
| `NO_COLOR`                      | all                     | Set to `1` to disable colored output                                       |

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
- **IP Threat Filter:** Optional blocking of known malicious IPs via [IPsum](https://github.com/stamparm/ipsum) threat intelligence (`--enable-ipsum`)
- **Geo-Blocking:** Optional country-based IP blocking via MaxMind [GeoLite2-City](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) database (`--enable-geo-blocker`)
- **Security Logging:** File-based audit log for suspicious activity (see below)

### IPsum threat intelligence filter

The broker can block known malicious IPs using the [IPsum](https://github.com/stamparm/ipsum) threat intelligence feed. This feature is **disabled by default** and must be enabled with `--enable-ipsum` or `GO_CONNECT_ENABLE_IPSUM=1`.

When enabled, the broker loads the feed from a local file (`ipsum.txt` in the working directory). If the file does not exist, it is downloaded automatically. If the download fails and no local file is available, the broker exits with an error.

IPs that appear on 3 or more blacklists are blocked before the handshake. This means blocked connections are rejected at the TCP level with no protocol overhead.

The feed is downloaded from:

```
https://raw.githubusercontent.com/stamparm/ipsum/master/ipsum.txt
```

A custom feed URL can be set with `GO_CONNECT_IPSUM_SOURCE`.

Unparseable lines in the feed are logged as warnings to the console (not to the security log files).

Only IPs meeting the threshold are stored in memory. IPv6 addresses are not filtered (IPsum covers IPv4 only).

### Geo-blocking (country filter)

The broker can block connections from specific countries using MaxMind's [GeoLite2-City](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data) database. This feature is **disabled by default** and must be enabled with `--enable-geo-blocker` or `GO_CONNECT_ENABLE_GEO_BLOCKER=1`.

When enabled, the broker reads `GeoLite2.mmdb` from the current working directory. A custom path can be set with `GO_CONNECT_GEO_DB` (relative paths are resolved from the working directory). The file is **not** downloaded automatically -- you must obtain it from MaxMind.

Blocked countries are specified as a comma-separated list of ISO 3166-1 alpha-2 codes via `--blocked-countries` or `GO_CONNECT_BLOCKED_COUNTRIES`. Codes are trimmed and matched case-insensitively.

Example:

```bash
# Block connections from Russia, China, and North Korea
./go-connect broker --enable-geo-blocker --blocked-countries="RU,CN,KP"
```

Connections from blocked countries are rejected before the handshake, with no protocol overhead. Both IPv4 and IPv6 addresses are checked.

Blocked connections are logged with the `GEOBLOCK` tag in the security log.

### Security logging

The broker writes security-relevant events to log files in the `logs/` directory (relative to the working directory). Log files use the date-based naming pattern `YYYYMMDD.logs.txt`.

Each log entry has the format:

```
[YYYYMMDD hh:mm:ss.zzz] [TAG] [SEVERITY]  MESSAGE
```

The following events are logged:

| Event                 | Tag        | What is logged                                                                                                                    |
| --------------------- | ---------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Blocked IP            | `IPBLOCK`  | Remote IP, port, and IPsum blacklist count. Connection rejected before handshake.                                                 |
| Blocked country       | `GEOBLOCK` | Remote IP, port, and ISO country code. Connection rejected before handshake.                                                      |
| Invalid auth payload  | `AUTH`     | Raw content as Base64 (max 256 bytes), remote IP and port. Detects bots trying services like SSH or HTTP against the broker port. |
| Invalid connection ID | `ROUTING`  | Remote IP, port, and the submitted connection ID value.                                                                           |
| Wrong passphrase      | `AUTH`     | Remote IP and port only. The passphrase value is never logged.                                                                    |

The logger is memory-optimized: files are opened in append mode for each write and closed immediately after. This avoids memory issues during high volumes of connection attempts.

## Architecture

```
go-connect/
├── main.go                    # Entry point
├── cmd/                       # CLI commands (broker, listen, connect, version)
├── pb/                        # Generated protobuf code
├── pkg/
│   ├── broker/                # Broker server, routing, client connections
│   ├── crypto/                # X25519 handshake + AES-256-GCM encryption
│   ├── geoblock/              # GeoLite2 country-based IP blocking
│   ├── ipsum/                 # IPsum threat intelligence feed parser and IP filter
│   ├── logging/               # File-based security logger
│   ├── protocol/              # Framing, encrypted sessions, handshake protocol
│   ├── tunnel/                # Listener, connector, and reconnect logic
│   └── ui/                    # Colored terminal output and network interface listing
├── proto/                     # Protobuf definitions
└── test/                      # Integration tests
```

## Testing

```bash
go test ./... -timeout 60s
```

## License

MIT License -- see [LICENSE](./LICENSE) for details.
