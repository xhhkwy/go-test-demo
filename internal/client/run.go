package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"banner-fingerprint/internal/config"
	"banner-fingerprint/internal/input"
	"banner-fingerprint/internal/model"
)

func Run(ctx context.Context, c config.Client, out io.Writer) error {
	file, err := os.Open(c.Input)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer file.Close()
	batch, err := input.Parse(file, model.DefaultMaxBodyBytes, model.DefaultMaxBatch, model.DefaultMaxBannerBytes)
	if err != nil {
		return fmt.Errorf("input: %w", err)
	}
	client := New(c.Server, c.Timeout)
	defer client.Close()
	results, err := client.Fingerprint(ctx, batch.Scans)
	if err != nil {
		return err
	}
	if c.Format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "IP\tPORT\tPROTOCOL\tPRODUCT\tVERSION\tOS_HINT\tCONFIDENCE"); err != nil {
		return err
	}
	show := func(s string) string {
		if s == "" {
			return "-"
		}
		return strings.Map(func(r rune) rune {
			if r < 32 || r == 127 {
				return ' '
			}
			return r
		}, s)
	}
	for _, r := range results {
		if _, err := fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%.2f\n", show(r.IP), r.Port, show(r.Protocol), show(r.Product), show(r.Version), show(r.OSHint), r.Confidence); err != nil {
			return err
		}
	}
	return w.Flush()
}
