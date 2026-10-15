package fingerprint

import (
	"bytes"
	"regexp"
	"strings"

	"banner-fingerprint/internal/model"
)

var httpStatus = regexp.MustCompile(`^HTTP/[0-9]+(?:\.[0-9]+)?[ \t]+[0-9]{3}(?:[ \t].*)?$`)

func features(data []byte) map[string]string {
	raw := string(data)
	f := map[string]string{"banner": raw}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	first := lines[0]
	if httpStatus.MatchString(first) {
		f["http_status"] = first
		for _, line := range lines[1:] {
			if line == "" {
				break
			}
			if line[0] == ' ' || line[0] == '\t' {
				continue
			}
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.EqualFold(key, "Server") {
				f["http_server"] = strings.TrimSpace(value)
				break
			}
		}
		return f
	}
	if strings.HasPrefix(first, "220 ") || strings.HasPrefix(first, "220-") {
		f["ftp"] = first
		return f
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "SSH-") {
			f["ssh"] = line
			break
		}
	}
	// Packet header: 3-byte little endian payload length, sequence=0.
	// A captured prefix is sufficient when it includes protocol 10 and the
	// complete NUL-terminated version; remaining handshake fields may be absent.
	if len(data) >= 6 && data[3] == 0 && data[4] == 10 {
		declared := int(data[0]) | int(data[1])<<8 | int(data[2])<<16
		end := bytes.IndexByte(data[5:], 0)
		if end > 0 && end <= 128 && declared >= end+2 {
			version := data[5 : 5+end]
			printable := true
			for _, b := range version {
				if b < 0x20 || b > 0x7e {
					printable = false
					break
				}
			}
			if printable {
				f["mysql_version"] = string(version)
			}
		}
	}
	return f
}

func (e *Engine) Recognize(s model.Scan, data []byte) model.Result {
	result := model.Unknown(s)
	f := features(data)
	for _, rule := range e.rules {
		text, ok := f[rule.Source]
		if !ok {
			continue
		}
		match := rule.re.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		result.Protocol = rule.Protocol
		result.Product = rule.Product
		result.Confidence = rule.Confidence
		if rule.VersionGroup > 0 {
			result.Version = match[rule.VersionGroup]
		}
		for _, os := range e.os {
			if text, ok := f[os.Source]; ok && os.re.MatchString(text) {
				result.OSHint = os.Hint
				break
			}
		}
		break
	}
	return result
}
