package fingerprint

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"banner-fingerprint/internal/model"
)

var ErrBannerTooLarge = errors.New("decoded banner exceeds limit")

func Decode(s model.Scan, limit int) ([]byte, error) {
	var data []byte
	switch s.BannerEncoding {
	case "", "text":
		if len(s.Banner) > limit {
			return nil, ErrBannerTooLarge
		}
		data = []byte(s.Banner)
	case "base64":
		if base64.StdEncoding.DecodedLen(len(s.Banner)) > limit+2 {
			return nil, ErrBannerTooLarge
		}
		var err error
		data, err = base64.StdEncoding.Strict().DecodeString(s.Banner)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 banner")
		}
	case "escaped":
		data = make([]byte, 0, min(len(s.Banner), limit))
		for i := 0; i < len(s.Banner); i++ {
			b := s.Banner[i]
			if b == '\\' {
				i++
				if i >= len(s.Banner) {
					return nil, fmt.Errorf("incomplete banner escape")
				}
				switch s.Banner[i] {
				case 'n':
					b = '\n'
				case 'r':
					b = '\r'
				case 't':
					b = '\t'
				case '\\':
					b = '\\'
				case '"':
					b = '"'
				case 'x':
					if i+2 >= len(s.Banner) {
						return nil, fmt.Errorf("incomplete hexadecimal escape")
					}
					n, err := strconv.ParseUint(s.Banner[i+1:i+3], 16, 8)
					if err != nil {
						return nil, fmt.Errorf("invalid hexadecimal escape")
					}
					b = byte(n)
					i += 2
				default:
					return nil, fmt.Errorf("unsupported banner escape")
				}
			}
			if len(data) == limit {
				return nil, ErrBannerTooLarge
			}
			data = append(data, b)
		}
	default:
		return nil, fmt.Errorf("unsupported banner_encoding")
	}
	if len(data) > limit {
		return nil, ErrBannerTooLarge
	}
	return data, nil
}
