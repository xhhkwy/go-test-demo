package fingerprint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func minimalRules() RuleFile {
	return RuleFile{Version: 1, Rules: []Rule{{ID: "test", Source: "banner", Pattern: "(test)", Protocol: "SSH", Product: "test", VersionGroup: 1, Priority: 1, Confidence: 0.9}}}
}
func TestRulesValidation(t *testing.T) {
	for name, mutate := range map[string]func(*RuleFile){
		"version":    func(f *RuleFile) { f.Version = 2 },
		"empty":      func(f *RuleFile) { f.Rules = nil },
		"duplicate":  func(f *RuleFile) { f.Rules = append(f.Rules, f.Rules[0]) },
		"regex":      func(f *RuleFile) { f.Rules[0].Pattern = "[" },
		"confidence": func(f *RuleFile) { f.Rules[0].Confidence = 1.1 },
		"negative":   func(f *RuleFile) { f.Rules[0].Confidence = -0.1 },
		"group":      func(f *RuleFile) { f.Rules[0].VersionGroup = 2 },
		"source":     func(f *RuleFile) { f.Rules[0].Source = "unsupported" },
		"protocol":   func(f *RuleFile) { f.Rules[0].Protocol = "other" },
		"os regex":   func(f *RuleFile) { f.OSHints = []OSRule{{ID: "os", Source: "ssh", Pattern: "[", Hint: "Linux"}} },
	} {
		t.Run(name, func(t *testing.T) {
			f := minimalRules()
			mutate(&f)
			data, _ := json.Marshal(f)
			if _, err := Compile(data); err == nil {
				t.Fatal("accepted invalid rules")
			}
		})
	}
	for _, data := range []string{`{}`, `{"version":1,"typo":true}`, "null", "[]", "bad", "{} {}"} {
		if _, err := Compile([]byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
}
func TestLoadRulesAndPriority(t *testing.T) {
	f := minimalRules()
	f.Rules = append(f.Rules, Rule{ID: "first", Source: "banner", Pattern: "x", Protocol: "FTP", Priority: 2, Confidence: 0.5})
	data, _ := json.Marshal(f)
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	e, err := Load(path)
	if err != nil || e.rules[0].ID != "first" {
		t.Fatalf("load: %v %+v", err, e)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file accepted")
	}
}
