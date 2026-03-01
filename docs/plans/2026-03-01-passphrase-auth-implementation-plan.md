# Passphrase Authentication Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add optional passphrase authentication to the broker so clients must prove they know the passphrase after the encrypted handshake, with silent connection closure on failure.

**Architecture:** New `Authenticate` protobuf message carries SHA-256(passphrase). After X25519 handshake, the broker reads an `Authenticate` message with a 10s timeout, compares the hash using `crypto/subtle.ConstantTimeCompare()`, and silently closes the connection on mismatch. Clients send the `Authenticate` message right after handshake, before `Register`/`ConnectRequest`. The passphrase is configured via `--passphrase` flag with `GO_CONNECT_PASSPHRASE` env var fallback on all three CLI commands.

**Tech Stack:** Go, Protocol Buffers, `crypto/sha256`, `crypto/subtle`

---

## Phase 1: Protocol Layer

### Task 1: Add Authenticate message to protobuf schema

**Files:**
- Modify: `proto/messages.proto:27-42`
- Regenerate: `pb/messages.pb.go`

**Step 1: Add the Authenticate message and envelope field**

In `proto/messages.proto`, add field 13 to the Envelope oneof and the new message at the end:

```protobuf
// In the Envelope oneof block, after line 40 (ErrorMsg error_msg = 12;):
    Authenticate authenticate = 13;

// After the ErrorMsg message definition at the end of the file:
message Authenticate {
  bytes passphrase_hash = 1;
}
```

The full Envelope oneof should look like:
```protobuf
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
    Authenticate authenticate = 13;
  }
}
```

**Step 2: Regenerate protobuf Go code**

Run: `PATH=$PATH:$HOME/go/bin protoc --go_out=. --go_opt=paths=source_relative proto/messages.proto`
Expected: `pb/messages.pb.go` regenerated with `Authenticate` message and `Envelope_Authenticate` wrapper.

**Step 3: Verify the generated code compiles**

Run: `cd /workspace && go build ./...`
Expected: Clean build, no errors.

---

## Phase 2: Broker Authentication

### Task 2: Add passphrase support to broker Server

**Files:**
- Modify: `pkg/broker/server.go:23-62` (imports, struct, constructor)

**Step 1: Write the failing test**

Add to `pkg/broker/server_test.go`. This test connects to a broker with a passphrase, sends the correct `Authenticate` message, then proceeds with `Register`. It should fail because `NewServer` doesn't accept a passphrase yet.

```go
func TestServer_PassphraseAuth_Correct(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithPassphrase("test-secret"))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	// Send correct passphrase hash.
	hash := sha256.Sum256([]byte("test-secret"))
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Authenticate: %v", err)
	}

	// Send Register — should succeed since passphrase was correct.
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "auth-test-id",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Register: %v", err)
	}

	ack, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive RegisterAck: %v", err)
	}
	regAck := ack.GetRegisterAck()
	if regAck == nil {
		t.Fatalf("expected RegisterAck, got %T", ack.GetPayload())
	}
	if !regAck.GetSuccess() {
		t.Fatalf("RegisterAck not successful: %s", regAck.GetMessage())
	}
}
```

Add required imports to the test file: `"crypto/sha256"`.

**Step 2: Run test to verify it fails**

Run: `cd /workspace && go test ./pkg/broker/ -run TestServer_PassphraseAuth_Correct -v`
Expected: FAIL — `WithPassphrase` is undefined.

**Step 3: Implement ServerOption pattern and passphrase field**

Modify `pkg/broker/server.go`:

Add `"crypto/sha256"`, `"crypto/subtle"`, `"time"` to imports.

Add `ServerOption` type and `WithPassphrase`:

```go
// ServerOption configures optional Server parameters.
type ServerOption func(*Server)

// WithPassphrase sets the passphrase that clients must provide
// after the encrypted handshake. An empty passphrase is valid and
// means no secret is required (clients still send SHA-256 of "").
func WithPassphrase(passphrase string) ServerOption {
	return func(s *Server) {
		hash := sha256.Sum256([]byte(passphrase))
		s.passphraseHash = hash[:]
	}
}
```

Add `passphraseHash []byte` field to the `Server` struct:

```go
type Server struct {
	address        string
	router         *Router
	listener       net.Listener
	clients        map[*ClientConn]struct{}
	clientMu       sync.Mutex
	connSem        chan struct{}
	wg             sync.WaitGroup
	closeCh        chan struct{}
	passphraseHash []byte
}
```

Change `NewServer` signature to accept options:

```go
func NewServer(address string, opts ...ServerOption) *Server {
	s := &Server{
		address: address,
		router:  NewRouter(),
		clients: make(map[*ClientConn]struct{}),
		connSem: make(chan struct{}, DefaultMaxConnections),
		closeCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(s)
	}
	// Default: hash of empty string (no passphrase required).
	if s.passphraseHash == nil {
		hash := sha256.Sum256([]byte(""))
		s.passphraseHash = hash[:]
	}
	return s
}
```

**Step 4: Run test to verify it still fails (Authenticate not handled yet)**

Run: `cd /workspace && go test ./pkg/broker/ -run TestServer_PassphraseAuth_Correct -v -timeout 30s`
Expected: FAIL or timeout — `handleClient` doesn't expect `Authenticate` as first message.

**Step 5: Implement authenticateClient in handleClient**

In `pkg/broker/server.go`, add the `authenticateClient` method:

```go
const (
	// AuthTimeout is the maximum time to wait for a client to send
	// the Authenticate message after the handshake completes.
	AuthTimeout = 10 * time.Second
)

// authenticateClient reads the Authenticate message from the client
// and verifies the passphrase hash. Returns nil on success. On failure,
// it silently closes the connection (no error message is sent to the
// client to prevent information leakage).
func (s *Server) authenticateClient(client *ClientConn) error {
	// Set a read deadline so clients cannot hold connections open
	// indefinitely without authenticating.
	if err := client.conn.SetReadDeadline(time.Now().Add(AuthTimeout)); err != nil {
		client.conn.Close()
		return fmt.Errorf("failed to set auth deadline: %w", err)
	}

	env, err := client.Receive()
	if err != nil {
		client.conn.Close()
		return fmt.Errorf("failed to receive auth message: %w", err)
	}

	// Clear the deadline for subsequent reads.
	if err := client.conn.SetReadDeadline(time.Time{}); err != nil {
		client.conn.Close()
		return fmt.Errorf("failed to clear auth deadline: %w", err)
	}

	auth := env.GetAuthenticate()
	if auth == nil {
		// Not an Authenticate message — silent close.
		client.conn.Close()
		return fmt.Errorf("expected Authenticate, got %T", env.GetPayload())
	}

	if subtle.ConstantTimeCompare(auth.GetPassphraseHash(), s.passphraseHash) != 1 {
		// Wrong passphrase — silent close, no response.
		client.conn.Close()
		return fmt.Errorf("passphrase mismatch")
	}

	return nil
}
```

Modify `handleClient` to call `authenticateClient` right after the handshake, before reading the role message:

```go
func (s *Server) handleClient(conn net.Conn) {
	client, err := NewClientConn(conn, s.router)
	if err != nil {
		log.Printf("broker: handshake failed: %v", err)
		conn.Close()
		return
	}

	s.trackClient(client)
	defer s.untrackClient(client)

	// Authenticate the client before determining its role.
	if err := s.authenticateClient(client); err != nil {
		log.Printf("broker: authentication failed: %v", err)
		return
	}

	// Read first message to determine client role.
	env, err := client.Receive()
	if err != nil {
		log.Printf("broker: failed to receive first message: %v", err)
		client.Close()
		return
	}

	client.UpdateLastSeen()

	switch {
	case env.GetRegister() != nil:
		connectionID := env.GetRegister().GetConnectionId()
		s.handleListener(client, connectionID)

	case env.GetConnectRequest() != nil:
		connectionID := env.GetConnectRequest().GetConnectionId()
		s.handleConnector(client, connectionID)

	default:
		log.Printf("broker: unexpected first message type: %T", env.GetPayload())
		_ = client.Send(&pb.Envelope{
			Payload: &pb.Envelope_ErrorMsg{
				ErrorMsg: &pb.ErrorMsg{
					Message: "expected Register or ConnectRequest as first message",
				},
			},
		})
		client.Close()
	}
}
```

**Step 6: Run test to verify it passes**

Run: `cd /workspace && go test ./pkg/broker/ -run TestServer_PassphraseAuth_Correct -v`
Expected: PASS.

### Task 3: Test wrong passphrase causes silent close

**Files:**
- Modify: `pkg/broker/server_test.go`

**Step 1: Write the failing test**

```go
func TestServer_PassphraseAuth_Wrong(t *testing.T) {
	srv := NewServer("127.0.0.1:0", WithPassphrase("correct-password"))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	// Send WRONG passphrase hash.
	hash := sha256.Sum256([]byte("wrong-password"))
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Authenticate: %v", err)
	}

	// The broker should close the connection silently.
	// Attempting to receive should fail with a read error.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err := session.Receive()
	if err == nil {
		t.Fatal("expected error on Receive after wrong passphrase, got nil")
	}
}
```

**Step 2: Run test to verify it passes**

Run: `cd /workspace && go test ./pkg/broker/ -run TestServer_PassphraseAuth_Wrong -v`
Expected: PASS — broker closes connection silently, `Receive` returns error.

### Task 4: Test no-passphrase broker still works

**Files:**
- Modify: `pkg/broker/server_test.go`

**Step 1: Write the test**

```go
func TestServer_PassphraseAuth_EmptyDefault(t *testing.T) {
	// Broker with no passphrase option — defaults to empty string.
	srv := NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	addr := srv.Address()
	conn, session := dialAndHandshake(t, addr)
	defer conn.Close()

	// Send hash of empty string (matching the default).
	hash := sha256.Sum256([]byte(""))
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Authenticate: %v", err)
	}

	// Should be able to Register successfully.
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: "empty-passphrase-test",
			},
		},
	}); err != nil {
		t.Fatalf("failed to send Register: %v", err)
	}

	ack, err := session.Receive()
	if err != nil {
		t.Fatalf("failed to receive RegisterAck: %v", err)
	}
	if !ack.GetRegisterAck().GetSuccess() {
		t.Fatalf("RegisterAck not successful: %s", ack.GetRegisterAck().GetMessage())
	}
}
```

**Step 2: Run test to verify it passes**

Run: `cd /workspace && go test ./pkg/broker/ -run TestServer_PassphraseAuth_EmptyDefault -v`
Expected: PASS.

### Task 5: Update existing broker test to send Authenticate

**Files:**
- Modify: `pkg/broker/server_test.go`

**Step 1: Update dialAndHandshake to optionally send Authenticate**

Replace the existing `dialAndHandshake` helper with a version that sends the Authenticate message (with empty passphrase by default):

```go
func dialAndHandshake(t *testing.T, addr string) (net.Conn, *protocol.Session) {
	return dialAndHandshakeWithPassphrase(t, addr, "")
}

func dialAndHandshakeWithPassphrase(t *testing.T, addr string, passphrase string) (net.Conn, *protocol.Session) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to dial broker at %s: %v", addr, err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		t.Fatalf("handshake failed: %v", err)
	}

	// Send Authenticate message with passphrase hash.
	hash := sha256.Sum256([]byte(passphrase))
	if err := session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	}); err != nil {
		conn.Close()
		t.Fatalf("failed to send Authenticate: %v", err)
	}

	return conn, session
}
```

**Step 2: Run all broker tests**

Run: `cd /workspace && go test ./pkg/broker/ -v`
Expected: ALL PASS — existing `TestServer_ListenerConnectorLink` works with the updated helper.

---

## Phase 3: Client Authentication

### Task 6: Add passphrase to Listener

**Files:**
- Modify: `pkg/tunnel/listener.go:63-138` (struct, constructor, Start method)

**Step 1: Add passphrase field and update constructor**

Add `passphrase string` field to `Listener` struct, update `NewListener` to accept passphrase:

```go
type Listener struct {
	localPort    string
	brokerAddr   string
	brokerConn   net.Conn
	session      *protocol.Session
	connectionID string
	passphrase   string
	streams      map[uint32]*streamState
	streamsMu    sync.Mutex
	closeCh      chan struct{}
	closeOnce    sync.Once
}

func NewListener(localPort, brokerAddr, connectionID, passphrase string) *Listener {
	return &Listener{
		localPort:    localPort,
		brokerAddr:   brokerAddr,
		connectionID: connectionID,
		passphrase:   passphrase,
		streams:      make(map[uint32]*streamState),
		closeCh:      make(chan struct{}),
	}
}
```

**Step 2: Send Authenticate after handshake in Start()**

In the `Start()` method, add the Authenticate send between the handshake and the Register send. Add `"crypto/sha256"` to imports:

```go
func (l *Listener) Start() error {
	conn, err := net.DialTimeout("tcp", l.brokerAddr, DialTimeout)
	if err != nil {
		return fmt.Errorf("listener: failed to dial broker at %s: %w", l.brokerAddr, err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("listener: handshake failed: %w", err)
	}

	l.brokerConn = conn
	l.session = session

	// Authenticate with the broker.
	hash := sha256.Sum256([]byte(l.passphrase))
	err = l.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("listener: failed to send authenticate: %w", err)
	}

	// Send Register message with the connection ID.
	err = l.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Register{
			Register: &pb.Register{
				ConnectionId: l.connectionID,
			},
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("listener: failed to send register: %w", err)
	}

	// ... rest unchanged (Receive RegisterAck, etc.)
```

**Step 3: Verify it compiles**

Run: `cd /workspace && go build ./...`
Expected: Compilation errors in `cmd/listen.go` and `test/integration_test.go` because `NewListener` now requires 4 args.

### Task 7: Add passphrase to Connector

**Files:**
- Modify: `pkg/tunnel/connector.go:40-130` (struct, constructor, Start method)

**Step 1: Add passphrase field and update constructor**

Add `passphrase string` field to `Connector` struct, update `NewConnector`:

```go
type Connector struct {
	brokerAddr     string
	connectionID   string
	localPort      string
	passphrase     string
	brokerConn     net.Conn
	session        *protocol.Session
	nextStreamID   atomic.Uint32
	streams        map[uint32]net.Conn
	streamsMu      sync.Mutex
	pendingStreams  map[uint32]chan bool
	pendingMu      sync.Mutex
	localListener  net.Listener
	closeCh        chan struct{}
	closeOnce      sync.Once
}

func NewConnector(brokerAddr, connectionID, localPort, passphrase string) *Connector {
	return &Connector{
		brokerAddr:    brokerAddr,
		connectionID:  connectionID,
		localPort:     localPort,
		passphrase:    passphrase,
		streams:       make(map[uint32]net.Conn),
		pendingStreams: make(map[uint32]chan bool),
		closeCh:       make(chan struct{}),
	}
}
```

**Step 2: Send Authenticate after handshake in Start()**

In the `Start()` method, add the Authenticate send between the handshake and the ConnectRequest send. Add `"crypto/sha256"` to imports:

```go
func (c *Connector) Start() error {
	conn, err := net.DialTimeout("tcp", c.brokerAddr, DialTimeout)
	if err != nil {
		return fmt.Errorf("connector: failed to dial broker at %s: %w", c.brokerAddr, err)
	}

	session, err := protocol.PerformHandshake(conn)
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: handshake failed: %w", err)
	}

	c.brokerConn = conn
	c.session = session

	// Authenticate with the broker.
	hash := sha256.Sum256([]byte(c.passphrase))
	err = c.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_Authenticate{
			Authenticate: &pb.Authenticate{
				PassphraseHash: hash[:],
			},
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: failed to send authenticate: %w", err)
	}

	// Send ConnectRequest with the connection ID.
	err = c.session.Send(&pb.Envelope{
		Payload: &pb.Envelope_ConnectRequest{
			ConnectRequest: &pb.ConnectRequest{
				ConnectionId: c.connectionID,
			},
		},
	})
	if err != nil {
		conn.Close()
		return fmt.Errorf("connector: failed to send connect request: %w", err)
	}

	// ... rest unchanged (Receive ConnectAck, etc.)
```

**Step 3: Verify it compiles**

Run: `cd /workspace && go build ./...`
Expected: Compilation errors in `cmd/connect.go` and `test/integration_test.go` because `NewConnector` now requires 4 args.

---

## Phase 4: CLI Updates

### Task 8: Update CLI commands to accept --passphrase flag

**Files:**
- Modify: `cmd/broker.go`
- Modify: `cmd/listen.go`
- Modify: `cmd/connect.go`

**Step 1: Update broker command**

In `cmd/broker.go`, change the command to use flags and pass the passphrase:

```go
func NewBrokerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "broker <address>",
		Short: "Start the broker server",
		Long:  "Starts the broker (Vermittler) that relays encrypted connections between clients",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			passphrase, _ := cmd.Flags().GetString("passphrase")
			if passphrase == "" {
				passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
			}

			srv := broker.NewServer(args[0], broker.WithPassphrase(passphrase))

			if err := srv.Start(); err != nil {
				return fmt.Errorf("failed to start broker: %w", err)
			}

			fmt.Printf("Broker listening on %s\n", srv.Address())

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			fmt.Println("\nShutting down broker...")
			srv.Stop()

			return nil
		},
	}

	cmd.Flags().String("passphrase", "", "passphrase for client authentication (overrides GO_CONNECT_PASSPHRASE env var)")

	return cmd
}
```

**Step 2: Update listen command**

In `cmd/listen.go`, read the passphrase and pass it to `NewListener`:

```go
// Inside RunE, after connectionID resolution:
passphrase, _ := cmd.Flags().GetString("passphrase")
if passphrase == "" {
	passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
}

listener := tunnel.NewListener(args[0], args[1], connectionID, passphrase)
```

Add the flag:
```go
cmd.Flags().String("passphrase", "", "passphrase for broker authentication (overrides GO_CONNECT_PASSPHRASE env var)")
```

**Step 3: Update connect command**

In `cmd/connect.go`, change to use a `cmd` variable and add the flag:

```go
func NewConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect <broker-address> <connection-id> <local-port>",
		Short: "Connect to a listener via the broker",
		Long:  "Connects to a listener through the broker and exposes the remote service on a local port",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			passphrase, _ := cmd.Flags().GetString("passphrase")
			if passphrase == "" {
				passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
			}

			connector := tunnel.NewConnector(args[0], args[1], args[2], passphrase)
			// ... rest unchanged
		},
	}

	cmd.Flags().String("passphrase", "", "passphrase for broker authentication (overrides GO_CONNECT_PASSPHRASE env var)")

	return cmd
}
```

**Step 4: Verify it compiles**

Run: `cd /workspace && go build ./...`
Expected: Compilation errors in `test/integration_test.go` only (next task).

---

## Phase 5: Fix Integration Tests

### Task 9: Update integration tests to send passphrase

**Files:**
- Modify: `test/integration_test.go`

**Step 1: Update all NewListener/NewConnector calls**

Add `""` as the fourth argument (passphrase) to every `NewListener` and `NewConnector` call in the integration tests:

```go
// In TestEndToEnd_EchoTunnel:
listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "")
connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, "")

// In TestEndToEnd_MultipleStreams:
listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "")
connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, "")

// In TestEndToEnd_DisconnectNotification:
listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "")
connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, "")
```

**Step 2: Verify everything compiles**

Run: `cd /workspace && go build ./...`
Expected: Clean build, no errors.

**Step 3: Run all existing tests**

Run: `cd /workspace && go test ./... -v -timeout 60s`
Expected: ALL PASS — no regressions.

---

## Phase 6: Integration Test with Passphrase

### Task 10: Add integration test for passphrase-protected tunnel

**Files:**
- Modify: `test/integration_test.go`

**Step 1: Write integration test with correct passphrase**

```go
func TestEndToEnd_EchoTunnel_WithPassphrase(t *testing.T) {
	const passphrase = "integration-test-secret"

	echoLn, echoPort := startEchoServer(t)
	defer echoLn.Close()

	srv := broker.NewServer("127.0.0.1:0", broker.WithPassphrase(passphrase))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	brokerAddr := srv.Address()
	connectionID := uuid.New().String()

	listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, passphrase)
	if err := listener.Start(); err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	time.Sleep(200 * time.Millisecond)

	connectorPort := getFreePort(t)
	connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, passphrase)
	if err := connector.Start(); err != nil {
		t.Fatalf("failed to start connector: %v", err)
	}
	defer connector.Close()

	time.Sleep(200 * time.Millisecond)

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", connectorPort))
	if err != nil {
		t.Fatalf("failed to connect to connector port: %v", err)
	}
	defer conn.Close()

	message := []byte("Hello through passphrase-protected tunnel!")
	_, err = conn.Write(message)
	if err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}

	response := make([]byte, len(message))
	_, err = io.ReadFull(conn, response)
	if err != nil {
		t.Fatalf("failed to read echo response: %v", err)
	}

	if string(response) != string(message) {
		t.Fatalf("echo mismatch: got %q, want %q", response, message)
	}
}
```

**Step 2: Write integration test for wrong passphrase rejection**

```go
func TestEndToEnd_WrongPassphrase_Rejected(t *testing.T) {
	srv := broker.NewServer("127.0.0.1:0", broker.WithPassphrase("correct-secret"))
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	brokerAddr := srv.Address()
	connectionID := uuid.New().String()

	echoLn, echoPort := startEchoServer(t)
	defer echoLn.Close()

	// Listener with wrong passphrase should fail to start.
	listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "wrong-secret")
	err := listener.Start()
	if err == nil {
		listener.Close()
		t.Fatal("expected listener.Start() to fail with wrong passphrase, got nil")
	}
}
```

**Step 3: Run all tests**

Run: `cd /workspace && go test ./... -v -timeout 60s`
Expected: ALL PASS.

---

## Phase 7: Final Verification

### Task 11: Run full test suite and build

**Step 1: Run all tests**

Run: `cd /workspace && go test ./... -v -timeout 60s`
Expected: ALL PASS.

**Step 2: Build the binary**

Run: `cd /workspace && go build -o go-connect .`
Expected: Binary builds successfully.

**Step 3: Verify CLI help shows passphrase flag**

Run: `cd /workspace && ./go-connect broker --help && ./go-connect listen --help && ./go-connect connect --help`
Expected: All three commands show `--passphrase` flag in help output.

---

## Phase 8: Documentation Updates

### Task 12: Update TASKS.md with milestone checklist

**Files:**
- Modify: `TASKS.md`

Create the milestone task checklist in `TASKS.md` with all phases and tasks listed, marking each as completed.

### Task 13: Update README.md with passphrase documentation

**Files:**
- Modify: `README.md`

Add documentation about the `--passphrase` flag and `GO_CONNECT_PASSPHRASE` environment variable to the usage examples in the README.

### Task 14: Mark milestone as complete in MILESTONE.md

**Files:**
- Modify: `MILESTONE.md`

Mark the milestone as completed.
