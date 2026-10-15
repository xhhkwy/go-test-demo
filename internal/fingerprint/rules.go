package fingerprint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
)

type Rule struct {
	ID           string  `json:"id"`
	Source       string  `json:"source"`
	Pattern      string  `json:"pattern"`
	Protocol     string  `json:"protocol"`
	Product      string  `json:"product"`
	VersionGroup int     `json:"version_group"`
	Priority     int     `json:"priority"`
	Confidence   float64 `json:"confidence"`
}
type OSRule struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Pattern string `json:"pattern"`
	Hint    string `json:"hint"`
}
type RuleFile struct {
	Version int      `json:"version"`
	Rules   []Rule   `json:"rules"`
	OSHints []OSRule `json:"os_hints"`
}
type compiledRule struct {
	Rule
	re *regexp.Regexp
}
type compiledOS struct {
	OSRule
	re *regexp.Regexp
}
type Engine struct {
	rules []compiledRule
	os    []compiledOS
}

func Load(path string) (*Engine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open rules: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("read rules: %w", err)
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("rules file exceeds 1 MiB")
	}
	return Compile(data)
}

func Compile(data []byte) (*Engine, error) {
	var file RuleFile
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("decode rules: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing rules data")
	}
	if file.Version != 1 || len(file.Rules) == 0 || len(file.Rules)+len(file.OSHints) > 1000 {
		return nil, fmt.Errorf("rules require version 1 and 1..1000 entries")
	}
	sources := map[string]bool{"banner": true, "ssh": true, "http_server": true, "http_status": true, "mysql_version": true, "ftp": true}
	protocols := map[string]bool{"SSH": true, "HTTP": true, "MySQL": true, "Redis": true, "FTP": true}
	ids := make(map[string]bool)
	check := func(id, source, pattern string) (*regexp.Regexp, error) {
		if id == "" || len(id) > 128 || ids[id] || !sources[source] || pattern == "" || len(pattern) > 8192 {
			return nil, fmt.Errorf("invalid or duplicate rule %q", id)
		}
		ids[id] = true
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", id, err)
		}
		return re, nil
	}
	e := &Engine{}
	for _, r := range file.Rules {
		re, err := check(r.ID, r.Source, r.Pattern)
		if err != nil {
			return nil, err
		}
		if !protocols[r.Protocol] || math.IsNaN(r.Confidence) || r.Confidence < 0 || r.Confidence > 1 || r.VersionGroup < 0 || r.VersionGroup > re.NumSubexp() {
			return nil, fmt.Errorf("invalid output or version group in rule %q", r.ID)
		}
		e.rules = append(e.rules, compiledRule{Rule: r, re: re})
	}
	sort.Slice(e.rules, func(i, j int) bool {
		a, b := e.rules[i], e.rules[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		return a.ID < b.ID
	})
	for _, r := range file.OSHints {
		re, err := check(r.ID, r.Source, r.Pattern)
		if err != nil {
			return nil, err
		}
		if r.Hint == "" {
			return nil, fmt.Errorf("empty hint in OS rule %q", r.ID)
		}
		e.os = append(e.os, compiledOS{OSRule: r, re: re})
	}
	return e, nil
}
