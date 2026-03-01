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
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mkloubert/go-connect/pkg/broker"
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
			bindFlag, _ := cmd.Flags().GetString("bind-to")
			address := parseBindAddress(bindFlag)

			passphrase, _ := cmd.Flags().GetString("passphrase")
			if passphrase == "" {
				passphrase = os.Getenv("GO_CONNECT_PASSPHRASE")
			}

			srv := broker.NewServer(address, broker.WithPassphrase(passphrase))

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

	cmd.Flags().String("bind-to", "0.0.0.0:1781", "address to listen on (host:port, :port, or host)")
	cmd.Flags().String("passphrase", "", "passphrase for client authentication (overrides GO_CONNECT_PASSPHRASE env var)")

	return cmd
}
