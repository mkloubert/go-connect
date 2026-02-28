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

package types

import (
	"github.com/mkloubert/go-connect/cmd"
	"github.com/spf13/cobra"
)

// AppContext stores the current application context.
type AppContext struct {
	rootCommand *cobra.Command
}

// NewAppContext creates a new instance of an AppContext object with all
// CLI subcommands registered.
func NewAppContext() *AppContext {
	rootCmd := &cobra.Command{
		Use:   "go-connect",
		Short: "Encrypted TCP tunnel tool",
		Long:  "Creates encrypted connections between clients via a broker over TCP/IP",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	rootCmd.AddCommand(
		cmd.NewBrokerCommand(),
		cmd.NewListenCommand(),
		cmd.NewConnectCommand(),
		cmd.NewVersionCommand(),
	)

	return &AppContext{rootCommand: rootCmd}
}

// RootCommand returns the root command.
func (a *AppContext) RootCommand() *cobra.Command {
	return a.rootCommand
}

// Run runs the application by executing the root command.
func (a *AppContext) Run() error {
	return a.rootCommand.Execute()
}
