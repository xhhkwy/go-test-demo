package config

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"time"

	"banner-fingerprint/internal/model"
)

type Server struct {
	Addr           string
	Rules          string
	MaxBodyBytes   int64
	MaxBatch       int
	MaxBannerBytes int
	MaxConcurrent  int
}

func ServerFlags(args []string, out io.Writer) (Server, error) {
	var c Server
	f := flag.NewFlagSet("server", flag.ContinueOnError)
	f.SetOutput(out)
	f.StringVar(&c.Addr, "addr", ":8080", "HTTP listen address")
	f.StringVar(&c.Rules, "rules", "rules/fingerprints.json", "external fingerprint rules")
	f.Int64Var(&c.MaxBodyBytes, "max-body-bytes", model.DefaultMaxBodyBytes, "maximum JSON request bytes")
	f.IntVar(&c.MaxBatch, "max-batch", model.DefaultMaxBatch, "maximum records per request")
	f.IntVar(&c.MaxBannerBytes, "max-banner-bytes", model.DefaultMaxBannerBytes, "maximum decoded banner bytes")
	f.IntVar(&c.MaxConcurrent, "max-concurrent", 4, "maximum concurrent fingerprint requests")
	if err := f.Parse(args); err != nil {
		return c, err
	}
	if f.NArg() != 0 {
		return c, fmt.Errorf("unexpected arguments: %v", f.Args())
	}
	_, port, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return c, fmt.Errorf("invalid --addr: %w", err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("invalid listen port")
	}
	if c.Rules == "" || c.MaxBodyBytes < 1 || c.MaxBatch < 1 || c.MaxBannerBytes < 1 || c.MaxConcurrent < 1 {
		return c, fmt.Errorf("rules path and positive limits are required")
	}
	return c, nil
}

type Client struct {
	Input   string
	Server  string
	Format  string
	Timeout time.Duration
}

func ClientFlags(args []string, out io.Writer) (Client, error) {
	var c Client
	f := flag.NewFlagSet("client", flag.ContinueOnError)
	f.SetOutput(out)
	f.StringVar(&c.Input, "input", "testdata/scans.json", "local scan JSON array")
	f.StringVar(&c.Server, "server", "http://127.0.0.1:8080", "server base URL")
	f.StringVar(&c.Format, "format", "table", "output format: table or json")
	f.DurationVar(&c.Timeout, "timeout", 30*time.Second, "request timeout")
	if err := f.Parse(args); err != nil {
		return c, err
	}
	if f.NArg() != 0 {
		return c, fmt.Errorf("unexpected arguments: %v", f.Args())
	}
	u, err := url.Parse(c.Server)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return c, fmt.Errorf("--server must be an HTTP(S) origin URL")
	}
	if c.Input == "" || c.Timeout <= 0 || (c.Format != "table" && c.Format != "json") {
		return c, fmt.Errorf("input path, positive timeout and table/json format are required")
	}
	return c, nil
}
