package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"banner-fingerprint/internal/client"
	"banner-fingerprint/internal/config"
)

func main() {
	c, err := config.ClientFlags(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err == nil {
		err = client.Run(context.Background(), c, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
