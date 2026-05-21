package apisec

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGenerateRawCommands(t *testing.T) {
	api := loadMinimalOpenAPI(t)
	parser := NewParser(api, nil)

	commands, err := parser.GenerateRawCommands()
	if err != nil {
		t.Fatalf("GenerateRawCommands() error = %v", err)
	}

	application := findCommand(commands, "application-api")
	if application == nil {
		t.Fatalf("application-api command not generated")
	}
	getCmd := findCommand(application.Commands(), "get")
	if getCmd == nil {
		t.Fatalf("application-api get command not generated")
	}
	if getCmd.Flags().Lookup("page") == nil {
		t.Fatalf("page flag not generated")
	}
	if !strings.Contains(getCmd.Long, "Endpoint: GET /api/ApplicationAPI") {
		t.Fatalf("help missing endpoint: %s", getCmd.Long)
	}
	if !strings.Contains(getCmd.Long, "Operation ID: ApplicationAPI_get") {
		t.Fatalf("help missing operation ID: %s", getCmd.Long)
	}

	postCmd := findCommand(application.Commands(), "post")
	if postCmd == nil {
		t.Fatalf("application-api post command not generated")
	}
	if postCmd.Flags().Lookup("body") == nil {
		t.Fatalf("body flag not generated")
	}
	if postCmd.Flags().Lookup("body-file") == nil {
		t.Fatalf("body-file flag not generated")
	}
}

func TestGenerateSemanticCommands(t *testing.T) {
	api := loadMinimalOpenAPI(t)
	api.Paths["/api/FilterAPI"] = PathItem{Get: &Operation{OperationID: "FilterAPI_get", Summary: "Filter data"}}
	mapping := &CLIMapping{Commands: []MappedCommand{
		{
			Path:        []string{"asset", "app", "list"},
			OperationID: "ApplicationAPI_get",
			Short:       "List APISec applications",
			Long:        "List applications with pagination.",
			Examples:    []string{"chaitin-cli apisec asset app list --page 1"},
			Flags: map[string]MappedFlag{
				"page": {Name: "page-number", Description: "Page number to fetch."},
			},
		},
		{
			Path:        []string{"risk", "event", "list"},
			OperationID: "FilterAPI_get",
			Short:       "List risk event groups",
			Query: map[string]string{
				"scope": "risk:risk_event",
			},
		},
	}}
	parser := NewParser(api, mapping)

	commands, err := parser.GenerateSemanticCommands()
	if err != nil {
		t.Fatalf("GenerateSemanticCommands() error = %v", err)
	}
	asset := findCommand(commands, "asset")
	if asset == nil {
		t.Fatalf("asset command not generated")
	}
	app := findCommand(asset.Commands(), "app")
	if app == nil {
		t.Fatalf("asset app command not generated")
	}
	list := findCommand(app.Commands(), "list")
	if list == nil {
		t.Fatalf("asset app list command not generated")
	}
	if list.Short != "List APISec applications" {
		t.Fatalf("Short = %q, want mapped short", list.Short)
	}
	if !strings.Contains(list.Long, "Operation ID: ApplicationAPI_get") {
		t.Fatalf("Long missing operation ID: %s", list.Long)
	}
	flag := list.Flags().Lookup("page-number")
	if flag == nil {
		t.Fatalf("mapped page-number flag not generated")
	}
	if flag.Usage != "Page number to fetch." {
		t.Fatalf("flag usage = %q, want mapped description", flag.Usage)
	}
	riskList := findCommandPath(commands, "risk", "event", "list")
	if riskList == nil {
		t.Fatalf("risk event list command not generated")
	}
	if !strings.Contains(riskList.Long, "Default query:") || !strings.Contains(riskList.Long, "scope=risk:risk_event") {
		t.Fatalf("risk event list help missing default query: %s", riskList.Long)
	}
	if riskList.Flags().Lookup("query") == nil {
		t.Fatalf("query flag not generated")
	}
}

func TestEmbeddedMappingCoversPriorityGroups(t *testing.T) {
	api, mapping, err := loadEmbeddedSchema()
	if err != nil {
		t.Fatalf("loadEmbeddedSchema() error = %v", err)
	}
	commands, err := NewParser(api, mapping).GenerateSemanticCommands()
	if err != nil {
		t.Fatalf("GenerateSemanticCommands() error = %v", err)
	}

	for _, path := range [][]string{
		{"asset", "api"},
		{"asset", "site"},
		{"asset", "app"},
		{"asset", "visitor"},
		{"asset", "config"},
		{"data", "rule"},
		{"risk", "config"},
		{"risk", "event"},
		{"risk", "vulnerability"},
	} {
		if findCommandPath(commands, path...) == nil {
			t.Fatalf("embedded mapping missing command path %v", path)
		}
	}
}

func TestEmbeddedAssetListCommandsHideInternalScopes(t *testing.T) {
	api, mapping, err := loadEmbeddedSchema()
	if err != nil {
		t.Fatalf("loadEmbeddedSchema() error = %v", err)
	}
	commands, err := NewParser(api, mapping).GenerateSemanticCommands()
	if err != nil {
		t.Fatalf("GenerateSemanticCommands() error = %v", err)
	}

	tests := []struct {
		name       string
		path       []string
		wantScope  string
		wantPhrase string
	}{
		{
			name:       "site list",
			path:       []string{"asset", "site", "list"},
			wantScope:  "scope=inventory:site",
			wantPhrase: "without passing the internal scope",
		},
		{
			name:       "api list",
			path:       []string{"asset", "api", "list"},
			wantScope:  "scope=inventory:api",
			wantPhrase: "without passing the internal scope",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := findCommandPath(commands, tt.path...)
			if cmd == nil {
				t.Fatalf("command path %v not generated", tt.path)
			}
			if !strings.Contains(cmd.Long, tt.wantScope) {
				t.Fatalf("help missing default scope %q:\n%s", tt.wantScope, cmd.Long)
			}
			if !strings.Contains(cmd.Long, tt.wantPhrase) {
				t.Fatalf("help missing user-facing guidance %q:\n%s", tt.wantPhrase, cmd.Long)
			}
		})
	}
}

func loadMinimalOpenAPI(t *testing.T) *OpenAPI {
	t.Helper()
	data, err := os.ReadFile("testdata/openapi_minimal.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	api, err := parseOpenAPI(data)
	if err != nil {
		t.Fatalf("parseOpenAPI() error = %v", err)
	}
	return api
}

func findCommand(commands []*cobra.Command, use string) *cobra.Command {
	for _, cmd := range commands {
		if cmd.Use == use {
			return cmd
		}
	}
	return nil
}

func findCommandPath(commands []*cobra.Command, path ...string) *cobra.Command {
	if len(path) == 0 {
		return nil
	}
	cmd := findCommand(commands, path[0])
	for _, segment := range path[1:] {
		if cmd == nil {
			return nil
		}
		cmd = findCommand(cmd.Commands(), segment)
	}
	return cmd
}
