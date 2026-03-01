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

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mkloubert/go-connect/pkg/tunnel"
	"github.com/spf13/cobra"
)

// NewConnectCommand creates a cobra command that connects to a listener
// via the broker. It opens a local TCP port where clients can connect,
// and tunnels all traffic through the broker to the remote listener.
func NewConnectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "connect",
		Aliases: []string{"c"},
		Short:   "Connect to a listener via the broker",
		Long:    "Connects to a listener through the broker and exposes the remote service on a local port",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := uiFromCmd(cmd)
			cfg := reconnectConfigFromCmd(cmd)

			brokerFlag, _ := cmd.Flags().GetString("broker")
			brokerAddr := parseBrokerAddress(brokerFlag)

			connectionID, _ := cmd.Flags().GetString("id")
			connectionID = strings.TrimSpace(connectionID)
			if connectionID == "" {
				connectionID = strings.TrimSpace(os.Getenv("GO_CONNECT_ID"))
			}
			if connectionID == "" {
				out.Error("Connection ID is required")
				out.Hint("Provide --id flag or set GO_CONNECT_ID environment variable.")
				return fmt.Errorf("--id flag or GO_CONNECT_ID environment variable is required")
			}

			port, _ := cmd.Flags().GetInt("port")
			localPort := strconv.Itoa(port)

			passphrase, _ := cmd.Flags().GetString("passphrase")
			if passphrase == "" {
				passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				out.BlankLine()
				out.Info("Shutting down connector...")
				cancel()
			}()

			var connector *tunnel.Connector
			firstConnect := true

			err := tunnel.RunWithReconnect(ctx, cfg,
				func(event tunnel.ReconnectEvent) {
					if event.Connected {
						out.Success("Re-established connection to broker")
						out.Success("Encryption re-negotiated")
						out.Success(fmt.Sprintf("Re-linked to listener %s", connectionID))
						return
					}
					if event.Attempt == 1 {
						out.Warning("Connection to broker lost")
						out.BlankLine()
						out.Info("Reconnecting...")
					}
					retryLabel := fmt.Sprintf("%d", event.MaxRetries)
					if event.MaxRetries == -1 {
						retryLabel = "\u221e"
					}
					out.Info(fmt.Sprintf("  Attempt %d/%s ... failed (waiting %s)",
						event.Attempt, retryLabel, event.Delay.Round(time.Millisecond)))
				},
				func() error {
					connector = tunnel.NewConnector(brokerAddr, connectionID, localPort, passphrase)
					if err := connector.Start(); err != nil {
						return err
					}
					if firstConnect {
						out.Success(fmt.Sprintf("Connected to broker at %s", brokerAddr))
						out.Success("Encryption established (X25519 + AES-256-GCM)")
						out.Success(fmt.Sprintf("Linked to listener %s", connectionID))
						out.BlankLine()
						out.Info(fmt.Sprintf("Local service available on 127.0.0.1:%s", localPort))
						firstConnect = false
					}
					return nil
				},
				func() <-chan struct{} {
					if connector != nil {
						return connector.Done()
					}
					ch := make(chan struct{})
					close(ch)
					return ch
				},
				func() {
					if connector != nil {
						connector.Close()
						connector = nil
					}
				},
			)

			if err != nil {
				if ctx.Err() != nil {
					return nil // User cancelled via Ctrl+C
				}
				errMsg := err.Error()
				if strings.Contains(errMsg, "no listener registered") {
					out.Error(fmt.Sprintf("No listener found for ID %q", connectionID))
					out.Hint("The listener may not have started yet, the ID may be incorrect, or the listener disconnected.")
					out.Hint(fmt.Sprintf("Run \"go-connect listen -p <port> -b %s\" on the remote machine first.", brokerAddr))
				} else if strings.Contains(errMsg, "connection refused") {
					out.Error(fmt.Sprintf("Cannot reach broker at %s", brokerAddr))
					out.Hint(fmt.Sprintf("Is the broker running? Start it with: go-connect broker --bind-to=%s", brokerAddr))
				} else if strings.Contains(errMsg, "passphrase mismatch") || strings.Contains(errMsg, "EOF") {
					out.Error("Authentication failed")
					out.Hint("The passphrase does not match. Check --passphrase or GO_CONNECT_PASSPHRASE.")
				} else {
					out.Error(fmt.Sprintf("Failed to connect: %s", errMsg))
				}
				return fmt.Errorf("failed to start connector: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringP("broker", "b", "127.0.0.1:1781", "broker address (host:port, :port, or host)")
	cmd.Flags().StringP("id", "i", "", "connection ID of the listener to connect to (overrides GO_CONNECT_ID env var)")
	cmd.Flags().IntP("port", "p", 12345, "local port to expose the service on")
	cmd.Flags().String("passphrase", "", "passphrase for broker authentication (overrides GO_CONNECT_PASSPHRASE env var)")

	return cmd
}
