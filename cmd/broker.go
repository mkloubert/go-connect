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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mkloubert/go-connect/pkg/broker"
	"github.com/mkloubert/go-connect/pkg/geoblock"
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

			enableSecurityLogs, _ := cmd.Flags().GetBool("enable-security-logs")
			if !enableSecurityLogs && os.Getenv("GO_CONNECT_ENABLE_SECURITY_LOGS") == "1" {
				enableSecurityLogs = true
			}

			var logger *logging.Logger
			if enableSecurityLogs {
				var err error
				logger, err = logging.NewLogger("logs")
				if err != nil {
					out.Error("Failed to initialize security logger")
					return fmt.Errorf("failed to create logger: %w", err)
				}
				out.Success("Security logging enabled (logs/)")
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

			enableGeoBlocker, _ := cmd.Flags().GetBool("enable-geo-blocker")
			if !enableGeoBlocker && os.Getenv("GO_CONNECT_ENABLE_GEO_BLOCKER") == "1" {
				enableGeoBlocker = true
			}

			var geoDB *geoblock.DB
			if enableGeoBlocker {
				countriesStr, _ := cmd.Flags().GetString("blocked-countries")
				if countriesStr == "" {
					countriesStr = os.Getenv("GO_CONNECT_BLOCKED_COUNTRIES")
				}

				var countries []string
				for _, c := range strings.Split(countriesStr, ",") {
					if trimmed := strings.TrimSpace(c); trimmed != "" {
						countries = append(countries, trimmed)
					}
				}

				if len(countries) == 0 {
					out.Error("No blocked countries specified")
					out.Hint("Use --blocked-countries or GO_CONNECT_BLOCKED_COUNTRIES to specify ISO country codes (e.g., 'RU,CN,KP').")
					return fmt.Errorf("geo-blocker enabled but no countries specified")
				}

				dbPath := strings.TrimSpace(os.Getenv("GO_CONNECT_GEO_DB"))
				if dbPath == "" {
					dbPath = geoblock.DefaultDBPath
				}
				if !filepath.IsAbs(dbPath) {
					dbPath, _ = filepath.Abs(dbPath)
				}

				var err error
				geoDB, err = geoblock.Open(dbPath, countries)
				if err != nil {
					out.Error(fmt.Sprintf("Failed to open GeoLite2 database: %v", err))
					out.Hint("Place a GeoLite2.mmdb file in the current directory or set GO_CONNECT_GEO_DB to the database path.")
					return fmt.Errorf("geo-block database required but unavailable: %w", err)
				}
				defer geoDB.Close()
				out.Success(fmt.Sprintf("Geo-blocker loaded: %d countries blocked", len(geoDB.Countries())))
				opts = append(opts, broker.WithGeoFilter(geoDB))
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
	cmd.Flags().Bool("enable-geo-blocker", false, "enable GeoLite2 country blocking (overrides GO_CONNECT_ENABLE_GEO_BLOCKER env var)")
	cmd.Flags().String("blocked-countries", "", "comma-separated ISO country codes to block (overrides GO_CONNECT_BLOCKED_COUNTRIES env var)")
	cmd.Flags().Bool("enable-security-logs", false, "enable security event logging (overrides GO_CONNECT_ENABLE_SECURITY_LOGS env var)")

	return cmd
}
