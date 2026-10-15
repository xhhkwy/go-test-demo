package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"banner-fingerprint/internal/model"
)

type Client struct {
	base string
	http *http.Client
}

func New(base string, timeout time.Duration) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			Transport: &http.Transport{
				DialContext:  (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				MaxIdleConns: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 60 * time.Second,
				TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 25 * time.Second,
			},
		},
	}
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Fingerprint(ctx context.Context, scans []model.Scan) ([]model.Result, error) {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(scans); err != nil {
		return nil, err
	}
	if int64(body.Len()) > model.DefaultMaxBodyBytes {
		return nil, fmt.Errorf("encoded request exceeds 8 MiB; reduce batch size")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/fingerprint", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fingerprint request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var api model.APIError
		if json.Unmarshal(data, &api) == nil && api.Error.Code != "" {
			return nil, fmt.Errorf("server HTTP %d (%s): %s", resp.StatusCode, api.Error.Code, api.Error.Message)
		}
		return nil, fmt.Errorf("server HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("response exceeds 16 MiB")
	}
	var results []model.Result
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&results); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if results == nil || len(results) != len(scans) {
		return nil, fmt.Errorf("server returned an invalid result count")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("server returned trailing JSON")
	}
	for i, r := range results {
		if r.IP != scans[i].IP || r.Port != scans[i].Port || r.Protocol == "" || math.IsNaN(r.Confidence) || r.Confidence < 0 || r.Confidence > 1 {
			return nil, fmt.Errorf("server returned an invalid result at index %d", i)
		}
	}
	return results, nil
}
