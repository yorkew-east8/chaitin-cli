package codeinsight

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chaitin/chaitin-cli/config"
	"gopkg.in/yaml.v3"
)

func TestNewCommandHelpShowsGroups(t *testing.T) {
	cmd := NewCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	help := out.String()
	for _, want := range []string{"project", "repo-config", "task", "--url", "--access-token"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q:\n%s", want, help)
		}
	}
}

func TestDryRunUsesEnvConfigAndRedactsSecrets(t *testing.T) {
	t.Setenv("CODEINSIGHT_URL", "https://ci.example.com")
	t.Setenv("CODEINSIGHT_TOKEN", "ci-secret")
	withRuntime(t, config.Raw{}, true)

	cmd := NewCommand()
	cmd.SetArgs([]string{
		"task", "create", "repo",
		"--project-name", "demo",
		"--task-name", "demo-repo",
		"--rule-set-name", "Corax-Java",
		"--repo-url", "https://git.example.com/group/demo.git",
		"--ref-name", "main",
		"--access-token", "repo-secret",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(out.String(), "ci-secret") || strings.Contains(out.String(), "repo-secret") {
		t.Fatalf("dry-run output leaked secret:\n%s", out.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("dry-run output is not JSON: %v\n%s", err, out.String())
	}
	if payload["dry_run"] != true {
		t.Fatalf("dry_run = %v, want true", payload["dry_run"])
	}
	requests := payload["requests"].([]any)
	req := requests[0].(map[string]any)
	if req["path"] != "/openapi/v1/createRepoOnlineScan" {
		t.Fatalf("path = %v", req["path"])
	}
}

func TestProjectCreateCreatesAndOutputsProjectID(t *testing.T) {
	var createdBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		switch r.URL.Path {
		case "/admin-api/scan/project/simple-project-list":
			if got := r.URL.Query().Get("projectName"); got != "demo-java" {
				t.Fatalf("projectName query = %q", got)
			}
			writeJSON(w, map[string]any{"code": 0, "data": []any{}})
		case "/admin-api/scan/project":
			decodeJSON(t, r.Body, &createdBody)
			writeJSON(w, map[string]any{"code": 0, "data": 1001})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	withRuntime(t, rawConfig(Config{URL: server.URL, AccessToken: "ci-token"}), false)

	cmd := NewCommand()
	cmd.SetArgs([]string{"project", "create", "--name", "demo-java", "--language", "java"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if createdBody["projectName"] != "demo-java" || toInt64(createdBody["languageId"]) != 3 {
		t.Fatalf("unexpected project body: %#v", createdBody)
	}
	var payload map[string]any
	decodeJSON(t, bytes.NewReader(out.Bytes()), &payload)
	if toInt64(payload["project_id"]) != 1001 || payload["created"] != true {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestTaskCreateLocalSendsMultipart(t *testing.T) {
	tmpDir := t.TempDir()
	productPath := filepath.Join(tmpDir, "demo.jar")
	sourcePath := filepath.Join(tmpDir, "src.zip")
	writeFile(t, productPath, "jar")
	writeFile(t, sourcePath, "src")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.URL.Path != "/openapi/v1/createOnlineScan" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := r.MultipartForm.Value["projectId"][0]; got != "1001" {
			t.Fatalf("projectId = %q", got)
		}
		assertMultipartFile(t, r.MultipartForm, "constructionProduct", "demo.jar")
		assertMultipartFile(t, r.MultipartForm, "sourcecodePackage", "src.zip")
		writeJSON(w, map[string]any{"code": 0, "data": 2002})
	}))
	defer server.Close()
	withRuntime(t, rawConfig(Config{URL: server.URL, AccessToken: "ci-token"}), false)

	cmd := NewCommand()
	cmd.SetArgs([]string{
		"task", "create", "local",
		"--project-id", "1001",
		"--task-name", "demo-local",
		"--rule-set-id", "16",
		"--construction-product", productPath,
		"--sourcecode-package", sourcePath,
		"--memory-limit", "8",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), `"task_id": 2002`) {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestRepoConfigCreateSavesSingleGitConfig(t *testing.T) {
	listCalls := 0
	var saveBody []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		switch r.URL.Path {
		case "/admin-api/scan/code/list":
			listCalls++
			if listCalls == 1 {
				writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"list": []any{}}})
				return
			}
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"list": []any{
				map[string]any{"id": 7, "name": "git-prod", "repoType": 1, "serverHost": "https://git.example.com/group/demo.git"},
			}}})
		case "/admin-api/scan/code/save":
			decodeJSON(t, r.Body, &saveBody)
			writeJSON(w, map[string]any{"code": 0, "data": true})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	withRuntime(t, rawConfig(Config{URL: server.URL, AccessToken: "ci-token"}), false)

	cmd := NewCommand()
	cmd.SetArgs([]string{
		"repo-config", "create",
		"--name", "git-prod",
		"--server-host", "https://git.example.com/group/demo.git",
		"--access-token", "repo-token",
		"--skip-check",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(saveBody) != 1 || saveBody[0]["name"] != "git-prod" || toInt64(saveBody[0]["authType"]) != 2 {
		t.Fatalf("unexpected save body: %#v", saveBody)
	}
	if !strings.Contains(out.String(), `"repo_config_id": 7`) {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestTaskCreateRepoSendsOpenAPIBody(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertAuth(t, r)
		if r.URL.Path != "/openapi/v1/createRepoOnlineScan" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		decodeJSON(t, r.Body, &body)
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{
			"taskId":    3003,
			"projectId": 1001,
			"taskName":  "demo-repo",
			"repoMode":  "direct_temporary",
			"repoType":  "git",
			"repoUrl":   "https://git.example.com/group/demo.git",
			"refType":   "branch",
			"refName":   "main",
		}})
	}))
	defer server.Close()
	withRuntime(t, rawConfig(Config{URL: server.URL, AccessToken: "ci-token"}), false)

	cmd := NewCommand()
	cmd.SetArgs([]string{
		"task", "create", "repo",
		"--project-name", "demo-java",
		"--task-name", "demo-repo",
		"--rule-set-name", "Corax-Java",
		"--repo-url", "https://git.example.com/group/demo.git",
		"--ref-name", "main",
		"--access-token", "repo-token",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if body["project_name"] != "demo-java" || body["rule_set_name"] != "Corax-Java" {
		t.Fatalf("unexpected body: %#v", body)
	}
	direct := body["direct_repo"].(map[string]any)
	if direct["access_token"] != "repo-token" {
		t.Fatalf("direct repo access token not set: %#v", direct)
	}
	if !strings.Contains(out.String(), `"task_id": 3003`) {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func withRuntime(t *testing.T, cfg config.Raw, isDryRun bool) {
	t.Helper()
	oldCfg := runtimeCfg
	oldDryRun := dryRun
	t.Cleanup(func() {
		runtimeCfg = oldCfg
		dryRun = oldDryRun
	})
	ApplyRuntimeConfig(NewCommand(), cfg, isDryRun)
}

func rawConfig(value Config) config.Raw {
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		panic(err)
	}
	return config.Raw{productName: node}
}

func assertAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer ci-token" {
		t.Fatalf("Authorization = %q", got)
	}
	if got := r.Header.Get("Token"); got != "ci-token" {
		t.Fatalf("Token = %q", got)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(t *testing.T, r io.Reader, target any) {
	t.Helper()
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func assertMultipartFile(t *testing.T, form *multipart.Form, field, filename string) {
	t.Helper()
	files := form.File[field]
	if len(files) != 1 {
		t.Fatalf("multipart field %s has %d files", field, len(files))
	}
	if files[0].Filename != filename {
		t.Fatalf("%s filename = %q, want %q", field, files[0].Filename, filename)
	}
}
