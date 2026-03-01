# UX & Robustness Improvements Design

**Date:** 2026-03-01
**Status:** Approved
**Focus:** User experience and connection robustness
**Target audience:** Broad (including non-technical users)

## Goals

1. Make CLI output clear, colorful, and informative
2. Provide actionable error messages with hints
3. Auto-reconnect on connection loss (critical requirement)
4. Show available broker IP addresses on startup
5. Support verbose/quiet/no-color output modes
6. Zero new external dependencies

## 1. Colored, Structured CLI Output

### New package: `pkg/ui`

Provides functions for categorized output:
- `Success(msg)` — green checkmark prefix
- `Error(msg)` — red cross prefix
- `Warning(msg)` — yellow warning prefix
- `Info(msg)` — plain info text
- `Hint(msg)` — indented hint text
- `Debug(msg)` — only shown with `--verbose`
- `Header(msg)` — section header
- `Bullet(msg)` — indented bullet point

### ANSI color support
- Own implementation, no external dependency
- Auto-detect TTY via `os.Stdout.Stat()` — no colors when piped/redirected
- `NO_COLOR` env var support (https://no-color.org)
- `--no-color` flag override

### Example outputs

**Broker:**
```
  ✓ Broker listening on 0.0.0.0:1781

  Available addresses for clients:
    • 192.168.1.42:1781     (eth0)
    • 10.0.0.5:1781         (docker0)
    • 172.16.0.1:1781       (wg0)
    • fd12::1:1781          (eth0, IPv6)
    • 127.0.0.1:1781        (lo, local only)

  Waiting for connections...
```

**Listener:**
```
  ✓ Connected to broker at 127.0.0.1:1781
  ✓ Encryption established (X25519 + AES-256-GCM)

  Connection ID: 327ac625-3b0c-4bd7-ab1b-bb9d733774ae

  Share this ID with the connecting client.
  Listening for connections on local port 5900...
```

**Connector:**
```
  ✓ Connected to broker at 127.0.0.1:1781
  ✓ Encryption established (X25519 + AES-256-GCM)
  ✓ Linked to listener 327ac625-3b0c-4bd7-ab1b-bb9d733774ae

  Local service available on 127.0.0.1:60000
```

**Broker bound to specific IP:**
```
  ✓ Broker listening on 192.168.1.42:1781

  Clients can connect with:
    go-connect listen  -b 192.168.1.42:1781 -p <port>
    go-connect connect -b 192.168.1.42:1781 -i <id> -p <port>

  Waiting for connections...
```

## 2. Actionable Error Messages

Each error includes the problem and a concrete hint for resolution.

| Situation | Message | Hint |
|-----------|---------|------|
| Broker unreachable | `✗ Cannot reach broker at X:Y` | "Is the broker running? Start it with: go-connect broker --bind-to=X:Y" |
| Wrong passphrase | `✗ Authentication failed` | "The passphrase does not match. Check --passphrase or GO_CONNECT_PASSPHRASE" |
| Listener not found | `✗ No listener found for ID "abc"` | "The listener may not have started yet, the ID may be incorrect, or the listener disconnected" + "Run go-connect listen -p <port> on the remote machine first" |
| Port in use | `✗ Port 60000 is already in use` | "Choose a different port with --port or stop the service using port 60000" |
| Local service unreachable | `✗ Cannot connect to local service on port 5900` | "Make sure the service is running on port 5900" |
| Handshake timeout | `✗ Connection timed out` | "The broker may be overloaded or unreachable. Try again later" |
| Connection lost | `⚠ Connection to broker lost` | "Reconnecting... (attempt N/M)" |

With `--verbose`, the original Go error is also displayed below the hint.

## 3. Auto-Reconnect with Exponential Backoff

### New file: `pkg/tunnel/reconnect.go`

Wraps the existing `Start()` function of listener and connector.

### Parameters

| Setting | Default | Flag | Env Var |
|---------|---------|------|---------|
| Max retries | 10 | `--max-retries` | `GO_CONNECT_MAX_RETRIES` |
| Initial interval | 1s | — | — |
| Max interval | 30s | — | — |
| Backoff factor | 2x | — | — |
| Jitter | ±25% | — | — |

### Behavior

- `--max-retries=0` disables reconnect (old behavior)
- `--max-retries=-1` means infinite retries
- Ctrl+C during reconnect cancels immediately
- Listener keeps its Connection ID across reconnects
- Connector re-links to the same listener ID
- Existing local streams are closed on reconnect (new ones can be opened)
- Each reconnect performs fresh X25519 handshake + new AES keys (security)
- Context-based cancellation for clean shutdown

### Output during reconnect

```
  ⚠ Connection to broker lost

  Reconnecting...
    Attempt 1/10 ... failed (waiting 1s)
    Attempt 2/10 ... failed (waiting 2s)
    Attempt 3/10 ... failed (waiting 4s)
    Attempt 4/10 ... connected!

  ✓ Re-established connection to broker
  ✓ Encryption re-negotiated
  ✓ Re-registered as listener (same Connection ID)
```

## 4. Verbose / Quiet / No-Color Modes

### New global flags (all commands)

| Flag | Short | Env Var | Effect |
|------|-------|---------|--------|
| `--verbose` | `-v` | `GO_CONNECT_VERBOSE=1` | Show technical details: handshake duration, nonce counters, stream IDs, raw errors |
| `--quiet` | `-q` | `GO_CONNECT_QUIET=1` | Only errors and essential info (Connection ID, port) |
| `--no-color` | — | `NO_COLOR=1` | No ANSI colors (automatic when not TTY) |

### Verbose example

```
$ go-connect listen -p 5900 -v
  ✓ Connected to broker at 127.0.0.1:1781 (12ms)
  ✓ X25519 handshake completed (3ms)
  ✓ AES-256-GCM session established
  ✓ Authenticated with broker

  Connection ID: 327ac625-3b0c-4bd7-ab1b-bb9d733774ae

  [DEBUG] Registered with broker, awaiting streams...
  [DEBUG] Heartbeat sent (seq=1)
  [DEBUG] Heartbeat received (seq=1, rtt=4ms)
  [DEBUG] OpenStream received (stream_id=1)
  [DEBUG] Dialing local port 5900... connected (2ms)
  [DEBUG] Stream 1: pumping data
```

### Quiet example

```
$ go-connect listen -p 5900 -q
327ac625-3b0c-4bd7-ab1b-bb9d733774ae
```

Quiet mode outputs only the Connection ID — ideal for scripting:
```bash
ID=$(go-connect listen -p 5900 -q &)
```

## 5. Broker IP Address Display

### New file: `pkg/ui/network.go`

Uses `net.Interfaces()` + `iface.Addrs()` to list available addresses.

### Behavior

- When bound to `0.0.0.0`: List all interfaces with IPv4 and IPv6 addresses
- When bound to specific IP: Show only that IP + usage examples
- Mark loopback with `(local only)`
- IPv6 addresses shown with interface name suffix
- In `--quiet` mode: Only "Listening on X:Y" (one line)

## File Changes Summary

### New files
| File | Purpose |
|------|---------|
| `pkg/ui/ui.go` | Colored output functions (Success, Error, Info, Hint, Debug, Header, Bullet) |
| `pkg/ui/network.go` | Network interface listing for broker |
| `pkg/tunnel/reconnect.go` | Auto-reconnect logic with exponential backoff |

### Modified files
| File | Change |
|------|--------|
| `cmd/broker.go` | UI output, IP display, global flags |
| `cmd/listen.go` | UI output, reconnect integration, global flags |
| `cmd/connect.go` | UI output, reconnect integration, global flags |
| `types/app_context.go` | Global flags (--verbose, --quiet, --no-color, --max-retries) |
| `pkg/tunnel/listener.go` | Make reconnect-capable, better error wrapping |
| `pkg/tunnel/connector.go` | Make reconnect-capable, better error wrapping |

### Unchanged
- Crypto layer (pkg/crypto/) — no changes
- Protocol layer (pkg/protocol/) — no changes
- Broker server logic (pkg/broker/) — only CLI output changes
- Protobuf schema (proto/) — no new messages

### No new dependencies
- Colors: Own ANSI implementation
- Interfaces: Go stdlib `net.Interfaces()`
- Reconnect: Own implementation

### Backwards compatibility
- All existing flags remain unchanged
- `--quiet` mode reproduces old behavior (minimal output)
- `--no-color` for CI/scripting
- `--max-retries=0` disables reconnect (old behavior)
