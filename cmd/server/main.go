package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"banner-fingerprint/internal/app"
	"banner-fingerprint/internal/config"
	"banner-fingerprint/internal/health"
)

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "healthcheck" {
		f := flag.NewFlagSet("server healthcheck", flag.ContinueOnError)
		f.SetOutput(out)
		url := f.String("url", "http://127.0.0.1:8080/health", "health endpoint")
		timeout := f.Duration("timeout", 2*time.Second, "probe timeout")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected healthcheck arguments")
		}
		return health.Probe(ctx, *url, *timeout)
	}
	c, err := config.ServerFlags(args, out)
	if err != nil {
		return err
	}
	return app.Serve(ctx, c, slog.New(slog.NewJSONHandler(out, nil)))
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := run(ctx, os.Args[1:], os.Stderr)
	if err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
