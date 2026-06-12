package ddr

import (
	"bytes"
	"testing"
)

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}

	if !bytes.Contains(stdout.Bytes(), []byte("DDR CLI")) {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestNewCommandDefaultsOutputToJSON(t *testing.T) {
	cmd := NewCommand()

	output, err := cmd.PersistentFlags().GetString("output")
	if err != nil {
		t.Fatalf("GetString(output) error = %v", err)
	}
	if output != "json" {
		t.Fatalf("output default = %q, want %q", output, "json")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}

	if _, ok := getRenderer(cmd).(*JSONRenderer); !ok {
		t.Fatalf("default renderer = %T, want *JSONRenderer", getRenderer(cmd))
	}
}
