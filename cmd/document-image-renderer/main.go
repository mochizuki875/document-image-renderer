package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mochizuki875/document-image-renderer/internal/cli"
)

func main() {
	// Cancel the context on Ctrl+C or SIGTERM so long conversions stop cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
