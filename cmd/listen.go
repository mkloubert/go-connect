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

	"github.com/google/uuid"
	"github.com/mkloubert/go-connect/pkg/tunnel"
	"github.com/spf13/cobra"
)

// NewListenCommand creates a cobra command that registers as a listener
// with the broker. It generates a random connection ID, connects to the
// broker, and forwards traffic to the specified local port.
func NewListenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "listen",
		Aliases: []string{"l"},
		Short:   "Register as listener with the broker",
		Long:    "Connects to the broker and registers as a listener, forwarding traffic to a local service",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := uiFromCmd(cmd)
			cfg := reconnectConfigFromCmd(cmd)

			port, _ := cmd.Flags().GetInt("port")
			localPort := strconv.Itoa(port)

			brokerFlag, _ := cmd.Flags().GetString("broker")
			brokerAddr := parseBrokerAddress(brokerFlag)

			connectionID, _ := cmd.Flags().GetString("id")
			connectionID = strings.TrimSpace(connectionID)
			if connectionID == "" {
				connectionID = strings.TrimSpace(os.Getenv("GO_CONNECT_ID"))
			}
			if connectionID == "" {
				connectionID = uuid.New().String()
			}

			passphrase, _ := cmd.Flags().GetString("passphrase")
			if passphrase == "" {
				passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Handle OS signals to trigger graceful shutdown.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				out.BlankLine()
				out.Info("Shutting down listener...")
				cancel()
			}()

			var listener *tunnel.Listener
			firstConnect := true

			err := tunnel.RunWithReconnect(ctx, cfg,
				func(event tunnel.ReconnectEvent) {
					if event.Connected {
						out.Success("Re-established connection to broker")
						out.Success("Encryption re-negotiated")
						out.Success(fmt.Sprintf("Re-registered as listener (Connection ID: %s)", connectionID))
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
					listener = tunnel.NewListener(localPort, brokerAddr, connectionID, passphrase)
					if err := listener.Start(); err != nil {
						return err
					}
					if firstConnect {
						out.Success(fmt.Sprintf("Connected to broker at %s", brokerAddr))
						out.Success("Encryption established (X25519 + AES-256-GCM)")
						out.BlankLine()

						if out.IsQuiet() {
							out.Raw(connectionID)
						} else {
							out.Info(fmt.Sprintf("Connection ID: %s", connectionID))
							out.BlankLine()
							out.Info("Share this ID with the connecting client.")
							out.Info(fmt.Sprintf("Listening for connections on local port %s...", localPort))
						}
						firstConnect = false
					}
					return nil
				},
				func() <-chan struct{} {
					if listener != nil {
						return listener.Done()
					}
					ch := make(chan struct{})
					close(ch)
					return ch
				},
				func() {
					if listener != nil {
						listener.Close()
						listener = nil
					}
				},
			)

			if err != nil {
				if ctx.Err() != nil {
					return nil // User cancelled via Ctrl+C
				}
				errMsg := err.Error()
				if strings.Contains(errMsg, "connection refused") {
					out.Error(fmt.Sprintf("Cannot reach broker at %s", brokerAddr))
					out.Hint(fmt.Sprintf("Is the broker running? Start it with: go-connect broker --bind-to=%s", brokerAddr))
				} else if strings.Contains(errMsg, "registration rejected") {
					out.Error("Registration rejected by broker")
					out.Hint("The connection ID may already be in use. Try a different --id.")
				} else if strings.Contains(errMsg, "passphrase mismatch") || strings.Contains(errMsg, "EOF") {
					out.Error("Authentication failed")
					out.Hint("The passphrase does not match. Check --passphrase or GO_CONNECT_PASSPHRASE.")
				} else {
					out.Error(fmt.Sprintf("Failed to connect: %s", errMsg))
				}
				return fmt.Errorf("failed to start listener: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().IntP("port", "p", 0, "local port of the service to expose (required)")
	_ = cmd.MarkFlagRequired("port")
	cmd.Flags().StringP("broker", "b", "127.0.0.1:1781", "broker address (host:port, :port, or host)")
	cmd.Flags().StringP("id", "i", "", "connection ID to use (overrides GO_CONNECT_ID env var; auto-generated if empty)")
	cmd.Flags().String("passphrase", "", "passphrase for broker authentication (overrides GO_CONNECT_PASSPHRASE env var)")

	return cmd
}
