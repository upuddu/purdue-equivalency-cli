// Command purdue-equivalency-cli queries Purdue University's Transfer Credit Course
// Equivalency Guide from the shell.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/upuddu/purdue-equivalency-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "purdue-equivalency-cli:", err)
		os.Exit(1)
	}
}
