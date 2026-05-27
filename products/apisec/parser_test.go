package apisec

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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

func TestSplitCreateBuildsBodyFromFlagsInDryRun(t *testing.T) {
	oldDryRun := dryRun
	dryRun = true
	t.Cleanup(func() { dryRun = oldDryRun })

	cmd := NewCommand()
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--url", "https://apisec.example", "asset", "split", "create", "--api-id", "a-1", "--origin", "query", "--key", "user_id", "--name", "按 user_id 拆分", "--disabled"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	stderr := captureStderr(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	for _, want := range []string{"\"original_api_id\": \"a-1\"", "\"origin\": \"1\"", "\"key\": \"user_id\"", "\"is_enabled\": false"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, stderr)
		}
	}
}

func TestRawFilterGetDoesNotUseSemanticDefaultScope(t *testing.T) {
	oldDryRun := dryRun
	dryRun = true
	t.Cleanup(func() { dryRun = oldDryRun })

	cmd := NewCommand()
	cmd.SetArgs([]string{"--url", "https://apisec.example", "raw", "filter-api", "get", "--query", "scope=risk:detect:strategy", "--query", "type__in=55"})

	stderr := captureStderr(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if strings.Contains(stderr, "inventory%3Aapp") || strings.Contains(stderr, "inventory:app") {
		t.Fatalf("raw filter-api get included semantic default scope:\n%s", stderr)
	}
	if got := strings.Count(stderr, "scope="); got != 1 {
		t.Fatalf("raw filter-api get scope count = %d, want 1:\n%s", got, stderr)
	}
	if !strings.Contains(stderr, "scope=risk%3Adetect%3Astrategy") {
		t.Fatalf("raw filter-api get missing explicit scope:\n%s", stderr)
	}
}

func TestSemanticListStillUsesDefaultScope(t *testing.T) {
	oldDryRun := dryRun
	dryRun = true
	t.Cleanup(func() { dryRun = oldDryRun })

	cmd := NewCommand()
	cmd.SetArgs([]string{"--url", "https://apisec.example", "risk", "event", "list"})

	stderr := captureStderr(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	})

	if !strings.Contains(stderr, "scope=risk%3Arisk_event") {
		t.Fatalf("semantic risk event list missing default scope:\n%s", stderr)
	}
}

func TestRollbackPlanPrintsWithoutRequest(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"risk", "strategy", "create-account-abuse-ip", "--rollback-plan"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Rollback", "risk:detect:strategy", "<created_id>", "--yes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rollback plan missing %q:\n%s", want, got)
		}
	}
}

func TestRawSplitPostUsesSchemaAndRollbackMetadata(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"raw", "split-config-api", "post", "--schema"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute schema error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"origin", "query=1", "body=4", "original_api_id", "按 user_id 拆分"} {
		if !strings.Contains(got, want) {
			t.Fatalf("raw schema missing %q:\n%s", want, got)
		}
	}

	out.Reset()
	cmd = NewCommand()
	cmd.SetArgs([]string{"raw", "split-config-api", "post", "--rollback-plan"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute rollback-plan error = %v", err)
	}
	got = out.String()
	for _, want := range []string{"inventory:detect:split", "<created_id>", "--yes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("raw rollback plan missing %q:\n%s", want, got)
		}
	}
}

func TestScopesCommandListsKnownScopes(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"scopes"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"inventory:detect:split", "risk:detect:strategy", "inventory:api"} {
		if !strings.Contains(got, want) {
			t.Fatalf("scopes output missing %q:\n%s", want, got)
		}
	}
}

func TestDangerousDeleteRequiresYes(t *testing.T) {
	cmd := NewCommand()
	cmd.SetArgs([]string{"asset", "split", "delete", "--body", `{"id__in":["s-1"]}`})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("Execute() error = %v, want --yes requirement", err)
	}
}

func TestCreateWithRollbackMetadataOutputsRollbackCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"err":null,"data":{"id":"54"},"msg":null}`))
	}))
	defer server.Close()

	cmd := NewCommand()
	cmd.SetArgs([]string{"--url", server.URL, "risk", "strategy", "create-account-abuse-ip", "--uuid", "s-1", "--src-ip-dis-cnt", "10"})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"\"action\":\"created\"", "\"id\":\"54\"", "risk:detect:strategy", "\"54\"", "--yes"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRawSplitCreateWithRollbackMetadataDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"err":null,"data":{"id":"4"},"msg":null}`))
	}))
	defer server.Close()

	cmd := NewCommand()
	cmd.SetArgs([]string{"--url", server.URL, "raw", "split-config-api", "post", "--body", `{"is_enabled":false,"name":"codex-final-split-test","original_api_id":"a-1","origin":1,"key":"codex_test_split_key"}`})
	var out bytes.Buffer
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"\"action\":\"created\"", "\"id\":\"4\"", "SplitConfigAPI_post", "inventory:detect:split"} {
		if !strings.Contains(got, want) {
			t.Fatalf("raw split create output missing %q:\n%s", want, got)
		}
	}
}

func TestCreateWrapperExtractsNestedID(t *testing.T) {
	result := map[string]any{
		"created": map[string]any{"uuid": "rs-9"},
	}
	mapped := &MappedCommand{
		Path: []string{"risk", "strategy", "create-account-abuse-ip"},
		Rollback: &RollbackHint{
			Command: `chaitin-cli apisec raw filter-api delete --body '{"scope":"risk:detect:strategy","id__in":["<created_id>"]}' --yes`,
		},
	}

	wrapped, ok := wrapMutationResult("POST", result, mapped)
	if !ok {
		t.Fatalf("wrapMutationResult ok = false, want true")
	}
	if wrapped["id"] != "rs-9" {
		t.Fatalf("id = %#v, want rs-9", wrapped["id"])
	}
	if !strings.Contains(wrapped["rollback"].(string), "rs-9") {
		t.Fatalf("rollback = %q, want nested id substituted", wrapped["rollback"])
	}
}

func TestCreateWrapperHandlesRawMetadataWithoutPath(t *testing.T) {
	mapped := &MappedCommand{
		OperationID: "SplitConfigAPI_post",
		Rollback: &RollbackHint{
			Command: `chaitin-cli apisec raw filter-api delete --body '{"scope":"inventory:detect:split","id__in":["<created_id>"]}' --yes`,
		},
	}

	wrapped, ok := wrapMutationResult("POST", map[string]any{"id": "4"}, mapped)
	if !ok {
		t.Fatalf("wrapMutationResult ok = false, want true")
	}
	if wrapped["resource"] != "SplitConfigAPI_post" {
		t.Fatalf("resource = %#v, want operation ID fallback", wrapped["resource"])
	}
	if wrapped["id"] != "4" {
		t.Fatalf("id = %#v, want 4", wrapped["id"])
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
		{"asset", "split"},
		{"asset", "visitor"},
		{"asset", "config"},
		{"data", "rule"},
		{"risk", "config"},
		{"risk", "strategy"},
		{"risk", "event"},
		{"risk", "vulnerability"},
	} {
		if findCommandPath(commands, path...) == nil {
			t.Fatalf("embedded mapping missing command path %v", path)
		}
	}
}

func TestEmbeddedPriorityCommandsExposeSchemaAndExamples(t *testing.T) {
	api, mapping, err := loadEmbeddedSchema()
	if err != nil {
		t.Fatalf("loadEmbeddedSchema() error = %v", err)
	}
	commands, err := NewParser(api, mapping).GenerateSemanticCommands()
	if err != nil {
		t.Fatalf("GenerateSemanticCommands() error = %v", err)
	}

	tests := []struct {
		path []string
		want []string
	}{
		{
			path: []string{"risk", "strategy", "create-account-abuse-ip"},
			want: []string{"--schema", "账号滥用 IP", "src_ip_dis_cnt", "effective_scope", "55"},
		},
		{
			path: []string{"asset", "split", "create"},
			want: []string{"--schema", "谓词拆分规则", "origin", "query=1", "original_api_id"},
		},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.path, " "), func(t *testing.T) {
			cmd := findCommandPath(commands, tt.path...)
			if cmd == nil {
				t.Fatalf("command path %v not generated", tt.path)
			}
			if cmd.Flags().Lookup("schema") == nil {
				t.Fatalf("schema flag not generated")
			}
			for _, want := range tt.want {
				if !strings.Contains(cmd.Long, want) {
					t.Fatalf("help missing %q:\n%s", want, cmd.Long)
				}
			}
		})
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
		wantOp     string
		wantScope  string
		wantPhrase string
	}{
		{
			name:       "app list",
			path:       []string{"asset", "app", "list"},
			wantOp:     "Operation ID: FilterAPI_get",
			wantScope:  "scope=inventory:app",
			wantPhrase: "without passing the internal scope",
		},
		{
			name:       "site list",
			path:       []string{"asset", "site", "list"},
			wantOp:     "Operation ID: FilterAPI_get",
			wantScope:  "scope=inventory:site",
			wantPhrase: "without passing the internal scope",
		},
		{
			name:       "api list",
			path:       []string{"asset", "api", "list"},
			wantOp:     "Operation ID: FilterAPI_get",
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
			if !strings.Contains(cmd.Long, tt.wantOp) {
				t.Fatalf("help missing operation %q:\n%s", tt.wantOp, cmd.Long)
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
