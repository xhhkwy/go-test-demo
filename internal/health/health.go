package health

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func Probe(ctx context.Context, url string, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("health timeout must be positive")
	}
	c := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer c.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("health request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(data) > 4096 {
		return fmt.Errorf("invalid health response size")
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &status); err != nil || status.Status != "ok" {
		return fmt.Errorf("invalid health response")
	}
	return nil
}
