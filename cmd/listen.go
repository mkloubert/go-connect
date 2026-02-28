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

	"github.com/google/uuid"
	"github.com/mkloubert/go-connect/pkg/tunnel"
	"github.com/spf13/cobra"
)

// NewListenCommand creates a cobra command that registers as a listener
// with the broker. It generates a random connection ID, connects to the
// broker, and forwards traffic to the specified local port.
func NewListenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "listen <local-port> <broker-address>",
		Short: "Register as listener with the broker",
		Long:  "Connects to the broker and registers as a listener, forwarding traffic to a local service",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			connectionID := uuid.New().String()

			listener := tunnel.NewListener(args[0], args[1], connectionID)
			if err := listener.Start(); err != nil {
				return fmt.Errorf("failed to start listener: %w", err)
			}

			fmt.Printf("Connection ID: %s\n", connectionID)
			fmt.Printf("Listening for connections on local port %s via broker %s\n", args[0], args[1])

			// Wait for signal or disconnect.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-sigCh:
				fmt.Println("\nShutting down listener...")
			case <-listener.Done():
				fmt.Println("Listener disconnected.")
			}

			listener.Close()

			return nil
		},
	}
}
