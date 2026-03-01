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
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mkloubert/go-connect/pkg/broker"
	"github.com/mkloubert/go-connect/pkg/ipsum"
	"github.com/mkloubert/go-connect/pkg/logging"
	"github.com/mkloubert/go-connect/pkg/ui"
	"github.com/spf13/cobra"
)

// NewBrokerCommand creates a cobra command that starts the broker server.
// The broker relays encrypted connections between listener and connector
// clients.
func NewBrokerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "broker",
		Aliases: []string{"b"},
		Short:   "Start the broker server",
		Long:    "Starts the broker (Vermittler) that relays encrypted connections between clients",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := uiFromCmd(cmd)

			bindFlag, _ := cmd.Flags().GetString("bind-to")
			address := parseBindAddress(bindFlag)

			passphrase, _ := cmd.Flags().GetString("passphrase")
			if passphrase == "" {
				passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
			}

			logger, err := logging.NewLogger("logs")
			if err != nil {
				out.Error("Failed to initialize security logger")
				return fmt.Errorf("failed to create logger: %w", err)
			}

			enableIPsum, _ := cmd.Flags().GetBool("enable-ipsum")
			if !enableIPsum && os.Getenv("GO_CONNECT_ENABLE_IPSUM") == "1" {
				enableIPsum = true
			}

			opts := []broker.ServerOption{
				broker.WithPassphrase(passphrase),
				broker.WithLogger(logger),
			}

			if enableIPsum {
				ipsumURL := strings.TrimSpace(os.Getenv("GO_CONNECT_IPSUM_SOURCE"))
				if ipsumURL == "" {
					ipsumURL = ipsum.DefaultURL
				}

				if _, statErr := os.Stat(ipsum.DefaultFilePath); os.IsNotExist(statErr) {
					out.Info("Downloading IPsum threat intelligence feed...")
					ipCtx, ipCancel := context.WithTimeout(context.Background(), 30*time.Second)
					dlErr := ipsum.DownloadToFile(ipCtx, ipsumURL, ipsum.DefaultFilePath)
					ipCancel()
					if dlErr != nil {
						out.Error(fmt.Sprintf("Failed to download IPsum feed: %v", dlErr))
						return fmt.Errorf("ipsum feed required but unavailable: %w", dlErr)
					}
					out.Success("IPsum feed downloaded to " + ipsum.DefaultFilePath)
				}

				ipFilter := ipsum.NewDB(ipsum.DefaultMinCount)
				warn := func(lineNum int, line string) {
					out.Warning(fmt.Sprintf("IPsum line %d: unparseable: %s", lineNum, line))
				}
				if err := ipFilter.LoadFromFile(ipsum.DefaultFilePath, warn); err != nil {
					out.Error(fmt.Sprintf("Failed to load IPsum feed: %v", err))
					return fmt.Errorf("ipsum feed required but unavailable: %w", err)
				}
				out.Success(fmt.Sprintf("IPsum loaded: %d IPs blocked (threshold >= %d)", ipFilter.Len(), ipsum.DefaultMinCount))
				opts = append(opts, broker.WithIPFilter(ipFilter))
			}

			srv := broker.NewServer(address, opts...)

			if err := srv.Start(); err != nil {
				out.Error(fmt.Sprintf("Failed to start broker on %s", address))
				out.Hint("Check that the port is not already in use and you have permission to bind to this address.")
				return fmt.Errorf("failed to start broker: %w", err)
			}

			out.Success(fmt.Sprintf("Broker listening on %s", srv.Address()))

			host, port, err := net.SplitHostPort(srv.Address())
			if err == nil && (host == "0.0.0.0" || host == "::") {
				out.BlankLine()
				out.Info("Available addresses for clients:")
				for _, addr := range ui.ListAddresses(port) {
					out.Bullet(fmt.Sprintf("%-24s (%s)", addr.Display(), addr.Label()))
				}
			} else if err == nil {
				out.BlankLine()
				out.Info("Clients can connect with:")
				out.Info(fmt.Sprintf("  go-connect listen  -b %s -p <port>", srv.Address()))
				out.Info(fmt.Sprintf("  go-connect connect -b %s -i <id> -p <port>", srv.Address()))
			}

			out.BlankLine()
			out.Info("Waiting for connections...")

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			out.BlankLine()
			out.Info("Shutting down broker...")
			srv.Stop()

			return nil
		},
	}

	cmd.Flags().String("bind-to", "0.0.0.0:1781", "address to listen on (host:port, :port, or host)")
	cmd.Flags().String("passphrase", "", "passphrase for client authentication (overrides GO_CONNECT_PASSPHRASE env var)")
	cmd.Flags().Bool("enable-ipsum", false, "enable IPsum IP blocking (overrides GO_CONNECT_ENABLE_IPSUM env var)")

	return cmd
}
