package apisec

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/chaitin/chaitin-cli/config"
	"gopkg.in/yaml.v3"
)

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()
	for _, name := range []string{"url", "api-token", "output", "verbose"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Fatalf("missing persistent flag --%s", name)
		}
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute help error = %v", err)
	}
	help := out.String()
	for _, want := range []string{"API-TOKEN", "raw", "--body", "--body-file"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestScopesCommandHonorsJSONOutput(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"--output", "json", "scopes"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var scopes []map[string]string
	if err := json.Unmarshal(out.Bytes(), &scopes); err != nil {
		t.Fatalf("scopes output is not JSON: %v\n%s", err, out.String())
	}
	if len(scopes) == 0 || scopes[0]["scope"] == "" {
		t.Fatalf("scopes output = %#v, want scope objects", scopes)
	}
}

func TestApplyRuntimeConfig(t *testing.T) {
	oldDryRun := dryRun
	t.Cleanup(func() { dryRun = oldDryRun })

	cmd := NewCommand()
	cfg := config.Raw{}
	var node yaml.Node
	if err := node.Encode(Config{URL: "https://apisec.example", APIToken: "token-1"}); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	cfg["apisec"] = node

	ApplyRuntimeConfig(cmd, cfg, true)
	cmd.PersistentPreRun(cmd, nil)

	if got, _ := cmd.PersistentFlags().GetString("url"); got != "https://apisec.example" {
		t.Fatalf("url = %q, want config value", got)
	}
	if got, _ := cmd.PersistentFlags().GetString("api-token"); got != "token-1" {
		t.Fatalf("api-token = %q, want config value", got)
	}
	if !dryRun {
		t.Fatalf("dryRun = false, want true")
	}
}
