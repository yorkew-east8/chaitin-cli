package monkeyscan

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chaitin/chaitin-cli/config"
	"gopkg.in/yaml.v3"
)

func TestResolveKeyPrefersEnvironment(t *testing.T) {
	oldCfg := runtimeCfg
	t.Cleanup(func() { runtimeCfg = oldCfg })
	runtimeCfg = Config{Key: "config-key"}
	t.Setenv(envKeyName, "env-key")

	key, source := resolveKey()
	if key != "env-key" || source != keySourceEnv {
		t.Fatalf("resolveKey() = %q, %q; want env-key, environment", key, source)
	}
}

func TestApplyRuntimeConfigUsesURLDefaultAndEnvOverride(t *testing.T) {
	oldCfg := runtimeCfg
	t.Cleanup(func() { runtimeCfg = oldCfg })
	t.Setenv(envURLName, "https://env.example.com/")

	raw := rawMonkeyscanConfig(t, Config{URL: "https://config.example.com", Key: "config-key"})
	ApplyRuntimeConfig(nil, raw, "config.yaml", false)
	applyRuntimeConfig()

	if runtimeCfg.URL != "https://env.example.com" {
		t.Fatalf("runtimeCfg.URL = %q, want env override", runtimeCfg.URL)
	}
}

func TestValidateReviewScope(t *testing.T) {
	tests := []struct {
		name    string
		scope   reviewScope
		wantErr string
	}{
		{name: "missing", wantErr: "--type 或 --all"},
		{name: "all with type", scope: reviewScope{All: true, Type: "staged"}, wantErr: "--all 不能和 --type"},
		{name: "committed missing base", scope: reviewScope{Type: "committed"}, wantErr: "必须且只能指定"},
		{name: "committed both base", scope: reviewScope{Type: "committed", Base: "main", BaseCommit: "abc"}, wantErr: "必须且只能指定"},
		{name: "staged with base", scope: reviewScope{Type: "staged", Base: "main"}, wantErr: "仅支持 --type committed"},
		{name: "staged", scope: reviewScope{Type: "staged"}},
		{name: "all", scope: reviewScope{All: true}},
		{name: "committed base", scope: reviewScope{Type: "committed", Base: "main"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReviewScope(tt.scope)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("validateReviewScope() error = %v", err)
			}
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("validateReviewScope() error = %v, want contains %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestClientHandlesStatusAndAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		switch r.URL.Path {
		case "/api/v1/cli/status":
			_ = json.NewEncoder(w).Encode(statusResponse{Authenticated: true, Account: accountInfo{Name: "alice"}, ReviewReady: true})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "missing"})
		}
	}))
	defer server.Close()

	client := newClient(server.URL, "secret")
	status, err := client.Status(t.Context())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if !status.Authenticated || status.Account.Name != "alice" || !status.ReviewReady {
		t.Fatalf("Status() = %+v", status)
	}
	_, err = client.ReviewDetail(t.Context(), "nope")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("ReviewDetail() error = %v, want missing", err)
	}
}

func TestAuthSetKeyAndClearPersistConfig(t *testing.T) {
	oldCfg, oldConfigPath, oldReadSecret := runtimeCfg, runtimeConfigPath, readSecret
	t.Cleanup(func() {
		runtimeCfg = oldCfg
		runtimeConfigPath = oldConfigPath
		readSecret = oldReadSecret
	})
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	runtimeCfg = Config{URL: "https://example.com"}
	runtimeConfigPath = configPath
	readSecret = func() (string, error) { return " saved-key\n", nil }

	cmd := NewCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "set-key"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set-key Execute() error = %v\n%s", err, out.String())
	}
	raw, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	saved, err := config.DecodeProduct[Config](raw, productName)
	if err != nil {
		t.Fatalf("DecodeProduct() error = %v", err)
	}
	if saved.Key != "saved-key" || saved.URL != "https://example.com" {
		t.Fatalf("saved config = %+v", saved)
	}

	cmd = NewCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"auth", "clear"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("clear Execute() error = %v\n%s", err, out.String())
	}
	raw, err = config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() after clear error = %v", err)
	}
	cleared, err := config.DecodeProduct[Config](raw, productName)
	if err != nil {
		t.Fatalf("DecodeProduct() after clear error = %v", err)
	}
	if cleared.Key != "" || cleared.URL != "https://example.com" {
		t.Fatalf("cleared config = %+v", cleared)
	}
}

func TestWriteReviewMarkdownIncludesFallbackDiff(t *testing.T) {
	path := filepath.Join(t.TempDir(), "review.md")
	detail := &reviewDetail{
		Run: reviewRun{ID: "run-1", RepositoryName: "acme/repo", Status: "completed"},
		Findings: []reviewFinding{{
			Title:          "问题",
			Severity:       "high",
			Recommendation: "修复",
			Location:       findingLocation{Path: "main.go", StartLine: 1, EndLine: 2},
		}},
	}
	if err := writeReviewMarkdown(path, detail); err != nil {
		t.Fatalf("writeReviewMarkdown() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	for _, want := range []string{"# MonkeyScan Review 结果", "推荐修复 Diff", "无", "main.go:1-2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("review.md missing %q:\n%s", want, got)
		}
	}
}

func TestRunReviewDryRunDoesNotRequireKeyOrHTTP(t *testing.T) {
	oldCfg, oldDryRun := runtimeCfg, dryRun
	t.Cleanup(func() {
		runtimeCfg = oldCfg
		dryRun = oldDryRun
	})
	dryRun = true
	runtimeCfg = Config{URL: "https://example.com"}

	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\nchanged\n")

	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	cmd := NewCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"review", "--type", "uncommitted"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "dry-run 未上传代码") {
		t.Fatalf("dry-run output unexpected:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(repo, reviewsDir)); !os.IsNotExist(err) {
		t.Fatalf("reviews dir exists after dry-run: %v", err)
	}
}

func TestRunReviewCreatesResultFiles(t *testing.T) {
	oldCfg, oldDryRun, oldPollInterval, oldPollTimeout := runtimeCfg, dryRun, pollInterval, pollTimeout
	t.Cleanup(func() {
		runtimeCfg = oldCfg
		dryRun = oldDryRun
		pollInterval = oldPollInterval
		pollTimeout = oldPollTimeout
	})
	pollInterval = time.Millisecond
	pollTimeout = time.Second

	var created bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/v1/cli/status":
			_ = json.NewEncoder(w).Encode(statusResponse{Authenticated: true, ReviewReady: true})
		case "/api/v1/cli/reviews/diff":
			created = true
			var req diffReviewRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if req.GitRemoteURL == "" || req.Diff == "" || len(req.Files) == 0 {
				t.Fatalf("request missing fields: %+v", req)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(diffReviewResponse{RunID: "run-1", TaskGroupID: "group-1", Status: "created"})
		case "/api/v1/cli/reviews/run-1":
			_ = json.NewEncoder(w).Encode(reviewDetail{
				Run:      reviewRun{ID: "run-1", RepositoryName: "acme/repo", Status: "completed", TaskGroupStatus: "success"},
				Findings: []reviewFinding{{Title: "问题", RecommendedDiff: "diff --git a/a b/a"}},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	runtimeCfg = Config{URL: server.URL, Key: "key"}

	repo := initGitRepo(t)
	writeFile(t, filepath.Join(repo, "README.md"), "hello\nchanged\n")

	wd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(repo); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	cmd := NewCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"review", "--type", "uncommitted"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	if !created {
		t.Fatal("CreateDiffReview was not called")
	}
	if !strings.Contains(out.String(), "Review 任务已提交") || !strings.Contains(out.String(), "结果文件") {
		t.Fatalf("output unexpected:\n%s", out.String())
	}
	matches, err := filepath.Glob(filepath.Join(repo, reviewsDir, "*", "review.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("review.md matches = %v, err = %v", matches, err)
	}
	statusMatches, _ := filepath.Glob(filepath.Join(repo, reviewsDir, "*", "status.json"))
	if len(statusMatches) != 1 {
		t.Fatalf("status.json matches = %v", statusMatches)
	}
}

func rawMonkeyscanConfig(t *testing.T, value Config) config.Raw {
	t.Helper()
	data, err := yaml.Marshal(map[string]Config{productName: value})
	if err != nil {
		t.Fatalf("Marshal YAML error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	raw, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return raw
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "remote", "add", "origin", "https://github.com/acme/repo.git")
	writeFile(t, filepath.Join(dir, "README.md"), "hello\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	out, err := gitOutput(t.Context(), dir, args...)
	if err != nil {
		t.Fatalf("git %v error = %v\n%s", args, err, out)
	}
}
