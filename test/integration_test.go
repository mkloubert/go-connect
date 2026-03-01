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

package test

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mkloubert/go-connect/pkg/broker"
	"github.com/mkloubert/go-connect/pkg/tunnel"
)

// startEchoServer starts a TCP server that echoes back any data it receives.
// It returns the listener and the port string. The caller must close the
// listener when done.
func startEchoServer(t *testing.T) (net.Listener, string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start echo server: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatalf("failed to parse echo server address: %v", err)
	}

	return ln, port
}

// getFreePort finds an available TCP port on localhost. It briefly opens and
// closes a listener to obtain a port number that is currently free.
func getFreePort(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatalf("failed to parse free port address: %v", err)
	}

	ln.Close()
	return port
}

// TestEndToEnd_EchoTunnel tests the full tunnel lifecycle:
// echo server -> listener -> broker -> connector -> test client.
// It sends a message through the tunnel and verifies the echo response.
func TestEndToEnd_EchoTunnel(t *testing.T) {
	// 1. Start echo server.
	echoLn, echoPort := startEchoServer(t)
	defer echoLn.Close()

	// 2. Start broker on a random port.
	srv := broker.NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	brokerAddr := srv.Address()
	connectionID := uuid.New().String()

	// 3. Start listener connecting echo port to broker.
	listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "")
	if err := listener.Start(); err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	// 4. Wait for registration to complete.
	time.Sleep(200 * time.Millisecond)

	// 5. Find a free port for the connector and start it.
	connectorPort := getFreePort(t)
	connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, "")
	if err := connector.Start(); err != nil {
		t.Fatalf("failed to start connector: %v", err)
	}
	defer connector.Close()

	// 6. Wait for connection setup.
	time.Sleep(200 * time.Millisecond)

	// 7. Connect to the connector's local port.
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", connectorPort))
	if err != nil {
		t.Fatalf("failed to connect to connector port: %v", err)
	}
	defer conn.Close()

	// 8. Send test message.
	message := []byte("Hello through the encrypted tunnel!")
	_, err = conn.Write(message)
	if err != nil {
		t.Fatalf("failed to write message: %v", err)
	}

	// 9. Read echo response with a timeout.
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("failed to set read deadline: %v", err)
	}

	response := make([]byte, len(message))
	_, err = io.ReadFull(conn, response)
	if err != nil {
		t.Fatalf("failed to read echo response: %v", err)
	}

	// 10. Verify response matches.
	if string(response) != string(message) {
		t.Fatalf("echo mismatch: got %q, want %q", response, message)
	}
}

// TestEndToEnd_MultipleStreams tests that multiple simultaneous TCP
// connections through the tunnel work correctly. Each connection sends
// unique data and verifies the correct echo response.
func TestEndToEnd_MultipleStreams(t *testing.T) {
	// Setup: echo server + broker + listener + connector.
	echoLn, echoPort := startEchoServer(t)
	defer echoLn.Close()

	srv := broker.NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	brokerAddr := srv.Address()
	connectionID := uuid.New().String()

	listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "")
	if err := listener.Start(); err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer listener.Close()

	time.Sleep(200 * time.Millisecond)

	connectorPort := getFreePort(t)
	connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, "")
	if err := connector.Start(); err != nil {
		t.Fatalf("failed to start connector: %v", err)
	}
	defer connector.Close()

	time.Sleep(200 * time.Millisecond)

	// Open 3 simultaneous connections.
	const numStreams = 3
	var wg sync.WaitGroup
	errors := make(chan error, numStreams)

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(streamIdx int) {
			defer wg.Done()

			conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", connectorPort))
			if err != nil {
				errors <- fmt.Errorf("stream %d: failed to connect: %w", streamIdx, err)
				return
			}
			defer conn.Close()

			// Each stream sends unique data.
			message := []byte(fmt.Sprintf("Stream %d: unique message data", streamIdx))
			_, err = conn.Write(message)
			if err != nil {
				errors <- fmt.Errorf("stream %d: failed to write: %w", streamIdx, err)
				return
			}

			if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				errors <- fmt.Errorf("stream %d: failed to set deadline: %w", streamIdx, err)
				return
			}

			response := make([]byte, len(message))
			_, err = io.ReadFull(conn, response)
			if err != nil {
				errors <- fmt.Errorf("stream %d: failed to read: %w", streamIdx, err)
				return
			}

			if string(response) != string(message) {
				errors <- fmt.Errorf("stream %d: echo mismatch: got %q, want %q", streamIdx, response, message)
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestEndToEnd_DisconnectNotification tests that when the listener
// disconnects, the connector is notified and its Done() channel fires.
func TestEndToEnd_DisconnectNotification(t *testing.T) {
	// Setup: broker + listener + connector (no echo server needed for
	// this test since we are testing disconnect propagation).
	srv := broker.NewServer("127.0.0.1:0")
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start broker: %v", err)
	}
	defer srv.Stop()

	brokerAddr := srv.Address()
	connectionID := uuid.New().String()

	// We need an echo server for the listener to register against,
	// even though we won't use it. The listener still needs a valid
	// local port parameter.
	echoLn, echoPort := startEchoServer(t)
	defer echoLn.Close()

	listener := tunnel.NewListener(echoPort, brokerAddr, connectionID, "")
	if err := listener.Start(); err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	connectorPort := getFreePort(t)
	connector := tunnel.NewConnector(brokerAddr, connectionID, connectorPort, "")
	if err := connector.Start(); err != nil {
		t.Fatalf("failed to start connector: %v", err)
	}
	defer connector.Close()

	time.Sleep(200 * time.Millisecond)

	// Close the listener -- this should trigger a disconnect
	// notification to the connector via the broker.
	listener.Close()

	// Verify the connector's Done() channel fires within a timeout.
	select {
	case <-connector.Done():
		// Success: connector was notified of disconnect.
	case <-time.After(5 * time.Second):
		t.Fatal("timeout: connector was not notified of listener disconnect within 5s")
	}
}

// TestEndToEnd_EchoTunnel_WithPassphrase tests the full tunnel lifecycle
// with a passphrase-protected broker. It verifies that a listener and
// connector with the correct passphrase can successfully exchange data
// through the encrypted tunnel.
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

// TestEndToEnd_WrongPassphrase_Rejected tests that a client providing an
// incorrect passphrase is rejected by the broker. The listener should fail
// to start because the broker closes the connection after authentication
// failure.
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
