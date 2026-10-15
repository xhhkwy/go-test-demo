package config

import (
	"bytes"
	"errors"
	"flag"
	"testing"
)

func TestDefaults(t *testing.T) {
	s, err := ServerFlags(nil, &bytes.Buffer{})
	if err != nil || s.Addr != ":8080" || s.MaxConcurrent != 4 {
		t.Fatalf("server defaults: %+v %v", s, err)
	}
	c, err := ClientFlags(nil, &bytes.Buffer{})
	if err != nil || c.Format != "table" || c.Timeout.Seconds() != 30 {
		t.Fatalf("client defaults: %+v %v", c, err)
	}
}

func TestHelp(t *testing.T) {
	for _, parse := range []func([]string, *bytes.Buffer) error{
		func(a []string, b *bytes.Buffer) error { _, e := ServerFlags(a, b); return e },
		func(a []string, b *bytes.Buffer) error { _, e := ClientFlags(a, b); return e },
	} {
		var out bytes.Buffer
		if err := parse([]string{"--help"}, &out); !errors.Is(err, flag.ErrHelp) || out.Len() == 0 {
			t.Fatalf("help: %v %q", err, out.String())
		}
	}
}

func TestInvalidConfiguration(t *testing.T) {
	for _, args := range [][]string{{"--addr", "bad"}, {"--addr", ":0"}, {"--max-batch", "0"}, {"--max-concurrent", "-1"}, {"extra"}} {
		if _, err := ServerFlags(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted server args: %v", args)
		}
	}
	for _, args := range [][]string{{"--server", "file:///tmp/x"}, {"--server", "http://host/path"}, {"--timeout", "0s"}, {"--format", "csv"}, {"extra"}} {
		if _, err := ClientFlags(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted client args: %v", args)
		}
	}
}
