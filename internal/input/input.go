package input

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"unicode/utf8"

	"banner-fingerprint/internal/fingerprint"
	"banner-fingerprint/internal/model"
)

type Error struct {
	Message  string
	Index    *int
	TooLarge bool
}

func (e *Error) Error() string {
	if e.Index != nil {
		return fmt.Sprintf("record[%d]: %s", *e.Index, e.Message)
	}
	return e.Message
}

type Batch struct {
	Scans   []model.Scan
	Banners [][]byte
}

func Parse(reader io.Reader, maxBytes int64, maxBatch, maxBanner int) (Batch, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) || int64(len(data)) > maxBytes {
		return Batch{}, &Error{Message: "request exceeds byte limit", TooLarge: true}
	}
	if err != nil {
		return Batch{}, &Error{Message: "cannot read input"}
	}
	if !utf8.Valid(data) {
		return Batch{}, &Error{Message: "JSON must be valid UTF-8; use base64 for binary data"}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return Batch{}, &Error{Message: "input must be one JSON array"}
	}
	batch := Batch{Scans: make([]model.Scan, 0), Banners: make([][]byte, 0)}
	for i := 0; decoder.More(); i++ {
		if i >= maxBatch {
			return Batch{}, &Error{Message: "batch exceeds record limit", TooLarge: true}
		}
		index := i
		fail := func(message string, tooLarge bool) (Batch, error) {
			return Batch{}, &Error{Message: message, Index: &index, TooLarge: tooLarge}
		}
		var r struct {
			IP       *string `json:"ip"`
			Port     *int    `json:"port"`
			Banner   *string `json:"banner"`
			Encoding string  `json:"banner_encoding"`
		}
		if err := decoder.Decode(&r); err != nil {
			var syntax *json.SyntaxError
			if errors.As(err, &syntax) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return Batch{}, &Error{Message: "invalid JSON syntax"}
			}
			return fail("invalid record fields or types", false)
		}
		if r.IP == nil || r.Port == nil || r.Banner == nil {
			return fail("ip, port and banner are required and cannot be null", false)
		}
		ip, err := netip.ParseAddr(*r.IP)
		if err != nil || ip.Zone() != "" {
			return fail("ip must be an IPv4 or IPv6 address without a zone", false)
		}
		if *r.Port < 1 || *r.Port > 65535 {
			return fail("port must be between 1 and 65535", false)
		}
		s := model.Scan{IP: *r.IP, Port: *r.Port, Banner: *r.Banner, BannerEncoding: r.Encoding}
		banner, err := fingerprint.Decode(s, maxBanner)
		if err != nil {
			return fail(err.Error(), errors.Is(err, fingerprint.ErrBannerTooLarge))
		}
		batch.Scans = append(batch.Scans, s)
		batch.Banners = append(batch.Banners, banner)
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim(']') {
		return Batch{}, &Error{Message: "unterminated JSON array"}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Batch{}, &Error{Message: "input must contain only one JSON array"}
	}
	return batch, nil
}
