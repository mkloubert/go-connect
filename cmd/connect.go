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

	"github.com/mkloubert/go-connect/pkg/tunnel"
	"github.com/spf13/cobra"
)

// NewConnectCommand creates a cobra command that connects to a listener
// via the broker. It opens a local TCP port where clients can connect,
// and tunnels all traffic through the broker to the remote listener.
func NewConnectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "connect <broker-address> <connection-id> <local-port>",
		Short: "Connect to a listener via the broker",
		Long:  "Connects to a listener through the broker and exposes the remote service on a local port",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			connector := tunnel.NewConnector(args[0], args[1], args[2])
			if err := connector.Start(); err != nil {
				return fmt.Errorf("failed to start connector: %w", err)
			}

			fmt.Printf("Connected to listener %s via broker %s\n", args[1], args[0])
			fmt.Printf("Local service available on port %s\n", args[2])

			// Wait for signal or disconnect.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-sigCh:
				fmt.Println("\nShutting down connector...")
			case <-connector.Done():
				fmt.Println("Connector disconnected.")
			}

			connector.Close()

			return nil
		},
	}
}
