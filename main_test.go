package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAppUsesDefaultConfigPath(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatalf("newApp() error = %v", err)
	}

	if app.configPath != defaultConfigPathFromCWD() {
		t.Fatalf("app.configPath = %q, want %q", app.configPath, defaultConfigPathFromCWD())
	}
}

func TestDryRunHelpMentionsRequestSummary(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatalf("newApp() error = %v", err)
	}
	flag := app.root.PersistentFlags().Lookup("dry-run")
	if flag == nil {
		t.Fatalf("missing --dry-run flag")
	}
	if !strings.Contains(flag.Usage, "request summary") {
		t.Fatalf("dry-run usage = %q, want request summary mention", flag.Usage)
	}
}

func TestCosmosDryRunFromRootDoesNotRequireNetwork(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("cosmos:\n  url: https://cosmos.example\n  api_key: secret-token-value\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	app, err := newApp()
	if err != nil {
		t.Fatalf("newApp() error = %v", err)
	}

	var out strings.Builder
	var errOut strings.Builder
	app.root.SetOut(&out)
	app.root.SetErr(&errOut)
	app.root.SetArgs([]string{
		"--dry-run",
		"-c", configPath,
		"cosmos", "asset", "save-host-asset",
		"--ip", "10.0.0.1/32",
		"--name", "dry-run-asset",
		"--organization_id", "1",
	})

	if err := app.execute(); err != nil {
		t.Fatalf("execute() error = %v\nstderr:\n%s", err, errOut.String())
	}

	got := out.String()
	for _, want := range []string{
		`"dry_run": true`,
		`"url": "https://cosmos.example/pedestal/rpc"`,
		`"method": "AssetService.SaveHostAsset"`,
		`"ip": "10.0.0.1/32"`,
		`"name": "dry-run-asset"`,
		`"organization_id": 1`,
		`"Authorization": "Bearer secr...alue"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret-token-value") {
		t.Fatalf("dry-run output leaked token:\n%s", got)
	}
}

func TestRootHelpIncludesCodeForce(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatalf("newApp() error = %v", err)
	}

	var out strings.Builder
	app.root.SetOut(&out)
	app.root.SetErr(&out)
	app.root.SetArgs([]string{"--help"})
	if err := app.root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "codeforce") {
		t.Fatalf("root help missing codeforce:\n%s", out.String())
	}
}

func TestRootHelpIncludesMonkeyScan(t *testing.T) {
	app, err := newApp()
	if err != nil {
		t.Fatalf("newApp() error = %v", err)
	}

	var out strings.Builder
	app.root.SetOut(&out)
	app.root.SetErr(&out)
	app.root.SetArgs([]string{"--help"})
	if err := app.root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "monkeyscan") {
		t.Fatalf("root help missing monkeyscan:\n%s", out.String())
	}
}

func TestEnsureRuntimeConfigLoaded(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom.yaml")
	if err := os.WriteFile(configPath, []byte("tanswer:\n  endpoint: https://example.com\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("Chdir() cleanup error = %v", err)
		}
	})

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	app := &app{configPath: configPath}
	if err := app.ensureRuntimeConfigLoaded(); err != nil {
		t.Fatalf("ensureRuntimeConfigLoaded() error = %v", err)
	}

	if !app.configLoaded {
		t.Fatal("configLoaded = false, want true")
	}
	if _, ok := app.config["tanswer"]; !ok {
		t.Fatal("config missing tanswer section")
	}
}

func TestEnsureRuntimeConfigLoadedWithMissingConfigFile(t *testing.T) {
	app := &app{configPath: filepath.Join(t.TempDir(), "missing.yaml")}
	if err := app.ensureRuntimeConfigLoaded(); err != nil {
		t.Fatalf("ensureRuntimeConfigLoaded() error = %v", err)
	}

	if !app.configLoaded {
		t.Fatal("configLoaded = false, want true")
	}
	if len(app.config) != 0 {
		t.Fatalf("len(config) = %d, want 0", len(app.config))
	}
}
