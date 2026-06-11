package codeinsight

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "codeinsight",
		Short: "CodeInsight project, repository config, scan task, and report management",
		Long: `CodeInsight CLI

Create CodeInsight projects, configure Git repository sources, create local or
remote scan tasks, query task results, and download exported reports.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			applyRuntimeConfig(cmd)
		},
	}

	cmd.PersistentFlags().String("url", "", "CodeInsight URL")
	cmd.PersistentFlags().String("access-token", "", "CodeInsight access token")
	cmd.PersistentFlags().String("token", "", "Alias for --access-token")
	_ = cmd.PersistentFlags().MarkHidden("token")
	cmd.PersistentFlags().Bool("insecure", true, "Skip TLS certificate verification")
	cmd.PersistentFlags().Int("timeout", defaultTimeoutSeconds, "HTTP request timeout in seconds")

	cmd.AddCommand(newProjectCommand())
	cmd.AddCommand(newRepoConfigCommand())
	cmd.AddCommand(newTaskCommand())
	return cmd
}

func newProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage CodeInsight projects",
	}
	cmd.AddCommand(newProjectCreateCommand())
	return cmd
}

func newProjectCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a CodeInsight project or reuse an existing project with the same name",
		RunE:  runProjectCreate,
	}
	cmd.Flags().String("name", "", "Project name")
	cmd.Flags().String("language", "", "Project language, for example java/go/python")
	cmd.Flags().Int64("language-id", 0, "Project language ID")
	return cmd
}

func runProjectCreate(cmd *cobra.Command, args []string) error {
	cfg, err := getConfigFromCommand(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("请提供 --name")
	}
	language, _ := cmd.Flags().GetString("language")
	languageIDFlag, _ := cmd.Flags().GetInt64("language-id")
	languageID, err := resolveLanguageID(language, languageIDFlag)
	if err != nil {
		return err
	}

	checkQuery := map[string]string{"projectName": name}
	createBody := map[string]any{
		"projectName":    name,
		"languageId":     languageID,
		"projectGroupId": 0,
		"type":           0,
		"members":        []any{},
	}
	if dryRun {
		return outputDryRun(cmd, []dryRunRequest{
			makeDryRunRequest(cfg, http.MethodGet, "/admin-api/scan/project/simple-project-list", checkQuery, nil, nil),
			makeDryRunRequest(cfg, http.MethodPost, "/admin-api/scan/project", nil, createBody, nil),
		})
	}

	client := NewClient(cfg)
	existing, err := client.findProjectByName(cmd.Context(), name)
	if err != nil {
		return err
	}
	if existing != nil {
		return outputOK(cmd, map[string]any{
			"created":      false,
			"reused":       true,
			"project_id":   existing.ID,
			"project_name": existing.Name,
			"message":      "命中同名项目，已复用现有项目",
		})
	}
	projectID, err := client.createProject(cmd.Context(), name, languageID)
	if err != nil {
		return err
	}
	return outputOK(cmd, map[string]any{
		"created":      true,
		"reused":       false,
		"project_id":   projectID,
		"project_name": name,
		"language_id":  languageID,
		"message":      "项目创建成功",
	})
}

func newRepoConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo-config",
		Short: "Manage CodeInsight code hosting repository configurations",
	}
	cmd.AddCommand(newRepoConfigCreateCommand())
	return cmd
}

func newRepoConfigCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Git repository configuration",
		RunE:  runRepoConfigCreate,
	}
	cmd.Flags().String("name", "", "Repository configuration name")
	cmd.Flags().String("repo-type", "git", "Repository type (git|svn|ftp|tfs)")
	cmd.Flags().String("config-mode", "single", "Git config mode (single|multi)")
	cmd.Flags().String("git-provider", "gitlab", "Git provider for Git configs (gitlab|github|gitee)")
	cmd.Flags().String("server-host", "", "Git platform URL or single repository URL")
	cmd.Flags().String("auth-type", "access_token", "Authentication type (username_password|access_token|anonymous)")
	cmd.Flags().String("account", "", "Account or username")
	cmd.Flags().String("username", "", "Display username field for compatibility")
	cmd.Flags().String("password", "", "Password for username_password auth")
	cmd.Flags().String("access-token", "", "Access token for repository platform auth")
	cmd.Flags().Bool("skip-check", false, "Skip Git connectivity check before saving")
	cmd.Flags().Bool("fetch-repositories", false, "Fetch remote repository list before saving a multi-repository config")
	cmd.Flags().String("repositories", "", "Comma-separated repository names, full names, or URLs for multi-repository config")
	return cmd
}

func runRepoConfigCreate(cmd *cobra.Command, args []string) error {
	cfg, err := getConfigFromCommand(cmd)
	if err != nil {
		return err
	}
	name, _ := cmd.Flags().GetString("name")
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("请提供 --name")
	}
	repoTypeText, _ := cmd.Flags().GetString("repo-type")
	repoType, err := repoTypeID(repoTypeText)
	if err != nil {
		return err
	}
	if repoType != 1 {
		return fmt.Errorf("首版 repo-config create 仅支持 Git 配置")
	}
	configModeText, _ := cmd.Flags().GetString("config-mode")
	configMode, err := parseConfigMode(configModeText)
	if err != nil {
		return err
	}
	gitProvider, _ := cmd.Flags().GetString("git-provider")
	gitProvider = strings.ToLower(strings.TrimSpace(gitProvider))
	if gitProvider != "gitlab" && gitProvider != "github" && gitProvider != "gitee" {
		return fmt.Errorf("git-provider 仅支持 gitlab、github、gitee")
	}
	serverHost, _ := cmd.Flags().GetString("server-host")
	serverHost = strings.TrimSpace(serverHost)
	if serverHost == "" {
		return fmt.Errorf("请提供 --server-host")
	}
	authTypeText, _ := cmd.Flags().GetString("auth-type")
	authType, err := authTypeID(authTypeText)
	if err != nil {
		return err
	}
	account, _ := cmd.Flags().GetString("account")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")
	repoAccessToken, _ := cmd.Flags().GetString("access-token")
	account = strings.TrimSpace(account)
	password = strings.TrimSpace(password)
	repoAccessToken = strings.TrimSpace(repoAccessToken)
	if authType == 1 && (account == "" || password == "") {
		return fmt.Errorf("用户名密码认证必须提供 --account 和 --password")
	}
	if authType == 2 && repoAccessToken == "" {
		return fmt.Errorf("Access Token 认证必须提供 --access-token")
	}
	if authType == 3 {
		account, password, repoAccessToken = "", "", ""
	}
	if configMode == repoConfigModeMulti && authType != 2 {
		return fmt.Errorf("多仓库配置当前仅支持 Access Token 登录")
	}
	repositories := splitCSV(mustGetString(cmd, "repositories"))
	if configMode == repoConfigModeMulti && len(repositories) == 0 {
		return fmt.Errorf("多仓库配置必须通过 --repositories 至少选择一个仓库")
	}

	checkBody := map[string]any{
		"serverHost":  serverHost,
		"authType":    authType,
		"account":     account,
		"accessToken": repoAccessToken,
	}
	savePayload := []map[string]any{{
		"name":        name,
		"repoType":    repoType,
		"configMode":  configMode,
		"gitProvider": gitProvider,
		"serverHost":  serverHost,
		"authType":    authType,
		"account":     account,
		"password":    password,
		"accessToken": repoAccessToken,
		"username":    username,
	}}
	if configMode == repoConfigModeMulti && !mustGetBool(cmd, "fetch-repositories") {
		savePayload[0]["repositories"] = buildRepositoriesFromNames(serverHost, repositories)
	}
	if dryRun {
		requests := []dryRunRequest{}
		if !mustGetBool(cmd, "skip-check") {
			requests = append(requests, makeDryRunRequest(cfg, http.MethodPost, "/admin-api/scan/code/check-git-configuration", nil, checkBody, nil))
		}
		if mustGetBool(cmd, "fetch-repositories") {
			requests = append(requests, makeDryRunRequest(cfg, http.MethodPost, "/admin-api/scan/code/list-remote-repositories", nil, map[string]any{
				"gitProvider": gitProvider,
				"serverHost":  serverHost,
				"authType":    authType,
				"account":     account,
				"accessToken": repoAccessToken,
			}, nil))
		}
		requests = append(requests, makeDryRunRequest(cfg, http.MethodPost, "/admin-api/scan/code/save", nil, savePayload, nil))
		return outputDryRun(cmd, requests)
	}

	client := NewClient(cfg)
	configs, err := client.listRepoConfigs(cmd.Context())
	if err != nil {
		return err
	}
	if existing := findRepoConfigByName(configs, name); existing != nil {
		return fmt.Errorf("已有同名配置：id=%d name=%s", existing.ID, existing.Name)
	}
	if !mustGetBool(cmd, "skip-check") {
		if err := client.checkGitConfiguration(cmd.Context(), checkBody); err != nil {
			return fmt.Errorf("Git 连通性校验失败: %w", err)
		}
	}
	if configMode == repoConfigModeMulti && mustGetBool(cmd, "fetch-repositories") {
		remoteRows, err := client.listRemoteRepositories(cmd.Context(), map[string]any{
			"gitProvider": gitProvider,
			"serverHost":  serverHost,
			"authType":    authType,
			"account":     account,
			"accessToken": repoAccessToken,
		})
		if err != nil {
			return err
		}
		selected, err := selectRemoteRepositories(remoteRows, repositories)
		if err != nil {
			return err
		}
		savePayload[0]["repositories"] = selected
	}
	if err := client.saveRepoConfig(cmd.Context(), savePayload); err != nil {
		return err
	}
	createdID := int64(0)
	configs, err = client.listRepoConfigs(cmd.Context())
	if err == nil {
		if created := findRepoConfigByName(configs, name); created != nil {
			createdID = created.ID
		}
	}
	return outputOK(cmd, map[string]any{
		"created":            true,
		"repo_config_id":     createdID,
		"repo_config_name":   name,
		"repo_type":          repoTypeName(repoType),
		"config_mode":        configModeText,
		"git_provider":       gitProvider,
		"repository_count":   len(repositories),
		"connectivity_check": !mustGetBool(cmd, "skip-check"),
	})
}

func newTaskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage CodeInsight scan tasks and results",
	}
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create scan tasks",
	}
	createCmd.AddCommand(newTaskCreateLocalCommand())
	createCmd.AddCommand(newTaskCreateRepoCommand())
	cmd.AddCommand(createCmd)
	cmd.AddCommand(newTaskResultCommand())
	return cmd
}

func newTaskCreateLocalCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "local",
		Short: "Create a local code package online scan task",
		RunE:  runTaskCreateLocal,
	}
	cmd.Flags().Int64("project-id", 0, "Project ID")
	cmd.Flags().String("task-name", "", "Task name")
	cmd.Flags().Int64("rule-set-id", 0, "Rule set ID")
	cmd.Flags().String("construction-product", "", "Construction product file to upload")
	cmd.Flags().String("sourcecode-package", "", "Source code package file to upload")
	cmd.Flags().String("command", "", "Custom scan command")
	cmd.Flags().Int("memory-limit", 0, "Memory limit in GB")
	cmd.Flags().String("worker-id", "", "Worker ID, for example __PLATFORM__")
	return cmd
}

func runTaskCreateLocal(cmd *cobra.Command, args []string) error {
	cfg, err := getConfigFromCommand(cmd)
	if err != nil {
		return err
	}
	projectID, _ := cmd.Flags().GetInt64("project-id")
	taskName := strings.TrimSpace(mustGetString(cmd, "task-name"))
	ruleSetID, _ := cmd.Flags().GetInt64("rule-set-id")
	constructionProduct := strings.TrimSpace(mustGetString(cmd, "construction-product"))
	sourcecodePackage := strings.TrimSpace(mustGetString(cmd, "sourcecode-package"))
	if projectID <= 0 {
		return fmt.Errorf("请提供 --project-id")
	}
	if taskName == "" {
		return fmt.Errorf("请提供 --task-name")
	}
	if ruleSetID <= 0 {
		return fmt.Errorf("请提供 --rule-set-id")
	}
	if constructionProduct == "" {
		return fmt.Errorf("未传递文件：请提供 --construction-product")
	}
	if err := validateFile(constructionProduct); err != nil {
		return err
	}
	if sourcecodePackage != "" {
		if err := validateFile(sourcecodePackage); err != nil {
			return err
		}
	}

	fields := map[string]string{
		"projectId":   strconv.FormatInt(projectID, 10),
		"taskName":    taskName,
		"ruleSetId":   strconv.FormatInt(ruleSetID, 10),
		"command":     mustGetString(cmd, "command"),
		"memoryLimit": intFlagString(cmd, "memory-limit"),
		"workerId":    mustGetString(cmd, "worker-id"),
	}
	files := map[string]string{"constructionProduct": constructionProduct}
	if sourcecodePackage != "" {
		files["sourcecodePackage"] = sourcecodePackage
	}
	if dryRun {
		return outputDryRun(cmd, []dryRunRequest{
			makeDryRunRequest(cfg, http.MethodPost, "/openapi/v1/createOnlineScan", nil, fieldsToAny(fields), files),
		})
	}

	taskID, err := NewClient(cfg).createOnlineScan(cmd.Context(), fields, files)
	if err != nil {
		return err
	}
	return outputOK(cmd, map[string]any{
		"task_id":                   taskID,
		"project_id":                projectID,
		"task_name":                 taskName,
		"rule_set_id":               ruleSetID,
		"construction_product":      basename(constructionProduct),
		"sourcecode_package":        basename(sourcecodePackage),
		"construction_product_sent": true,
		"sourcecode_package_sent":   sourcecodePackage != "",
		"status":                    "created",
	})
}

func newTaskCreateRepoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Create a remote repository online scan task",
		RunE:  runTaskCreateRepo,
	}
	cmd.Flags().String("project-name", "", "Project name")
	cmd.Flags().Int64("rule-set-id", 0, "Rule set ID")
	cmd.Flags().String("rule-set-name", "", "Rule set name")
	cmd.Flags().String("task-name", "", "Task name")
	cmd.Flags().String("repo-config-name", "", "Saved repository configuration name")
	cmd.Flags().String("repo-type", "git", "Direct repository type (git|svn|ftp|tfs)")
	cmd.Flags().String("repo-url", "", "Direct repository URL")
	cmd.Flags().String("auth-type", "access_token", "Direct repository auth type (username_password|access_token|anonymous)")
	cmd.Flags().String("username", "", "Direct repository username")
	cmd.Flags().String("password", "", "Direct repository password")
	cmd.Flags().String("access-token", "", "Direct repository access token")
	cmd.Flags().Bool("persist-repo-config", false, "Persist direct repository as a reusable config")
	cmd.Flags().String("persist-repo-config-name", "", "Repository config name when persisting direct repository")
	cmd.Flags().String("ref-type", "branch", "Git/TFS ref type (branch|tag)")
	cmd.Flags().String("ref-name", "", "Git/TFS branch or tag name")
	cmd.Flags().String("custom-scan-params", "", "Custom scan parameters")
	cmd.Flags().Int("need-command", -1, "0=custom scan params, 1=default scan params")
	cmd.Flags().Int("code-metrics", -1, "0=off, 1=on")
	cmd.Flags().String("rule-configuration-set-name", "", "Rule configuration set name")
	cmd.Flags().String("black-and-white-list-name", "", "Black/white list name")
	cmd.Flags().String("compiler-env-path", "", "JDK version or compiler environment path")
	cmd.Flags().String("build-command", "", "Build command")
	cmd.Flags().Int("memory-limit", 0, "Memory limit in GB")
	cmd.Flags().String("worker-id", "", "Worker ID")
	cmd.Flags().String("java-repo-config-name", "", "Java repository mirror config name")
	cmd.Flags().Int("incremental-analysis", -1, "0=off, 1=on")
	cmd.Flags().String("incremental-base-type", "", "Incremental baseline type (branch|tag|commitid)")
	cmd.Flags().String("incremental-base-ref", "", "Incremental baseline reference")
	cmd.Flags().Int("clone-detection", -1, "0=off, 1=on")
	cmd.Flags().Int("code-clone-granularity", 0, "Clone detection granularity")
	cmd.Flags().Int("architecture-analysis", -1, "0=off, 1=on")
	cmd.Flags().Bool("api-asset-inventory", false, "Enable API asset inventory")
	cmd.Flags().String("ai-judgment-bug-levels", "", "Comma-separated levels high,medium,low")
	cmd.Flags().String("deep-analysis-bug-levels", "", "Comma-separated levels high,medium,low")
	return cmd
}

func runTaskCreateRepo(cmd *cobra.Command, args []string) error {
	cfg, err := getConfigFromCommand(cmd)
	if err != nil {
		return err
	}
	projectName := strings.TrimSpace(mustGetString(cmd, "project-name"))
	taskName := strings.TrimSpace(mustGetString(cmd, "task-name"))
	if projectName == "" {
		return fmt.Errorf("请提供 --project-name")
	}
	if taskName == "" {
		return fmt.Errorf("请提供 --task-name")
	}
	ruleSetID, _ := cmd.Flags().GetInt64("rule-set-id")
	ruleSetName := strings.TrimSpace(mustGetString(cmd, "rule-set-name"))
	if ruleSetID <= 0 && ruleSetName == "" {
		return fmt.Errorf("rule-set-id 和 rule-set-name 必须二选一")
	}
	repoConfigName := strings.TrimSpace(mustGetString(cmd, "repo-config-name"))
	repoURL := strings.TrimSpace(mustGetString(cmd, "repo-url"))
	if repoConfigName != "" && repoURL != "" {
		return fmt.Errorf("仓库配置引用模式和直传模式不可同时使用")
	}
	if repoConfigName == "" && repoURL == "" {
		return fmt.Errorf("请提供 --repo-config-name 或 --repo-url")
	}
	body, err := buildRepoTaskBody(cmd, projectName, taskName, ruleSetID, ruleSetName, repoConfigName, repoURL)
	if err != nil {
		return err
	}
	if dryRun {
		return outputDryRun(cmd, []dryRunRequest{
			makeDryRunRequest(cfg, http.MethodPost, "/openapi/v1/createRepoOnlineScan", nil, body, nil),
		})
	}

	data, err := NewClient(cfg).createRepoOnlineScan(cmd.Context(), body)
	if err != nil {
		return err
	}
	return outputOK(cmd, map[string]any{
		"task_id":        int64Value(data, "taskId", "task_id"),
		"project_id":     int64Value(data, "projectId", "project_id"),
		"task_name":      stringValue(data, "taskName", "task_name"),
		"repo_mode":      stringValue(data, "repoMode", "repo_mode"),
		"repo_type":      stringValue(data, "repoType", "repo_type"),
		"repo_url":       stringValue(data, "repoUrl", "repo_url"),
		"repo_config_id": int64Value(data, "repoConfigId", "repo_config_id"),
		"ref_type":       stringValue(data, "refType", "ref_type"),
		"ref_name":       stringValue(data, "refName", "ref_name"),
		"rule_set_id":    ruleSetID,
		"rule_set_name":  ruleSetName,
		"status":         "created",
		"raw":            data,
	})
}

func newTaskResultCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "result",
		Short: "Show task result/status summary",
		RunE:  runTaskResult,
	}
	cmd.Flags().Int64("task-id", 0, "Task ID")
	cmd.Flags().Int64("project-id", 0, "Project ID for OpenAPI getTaskInfo")
	cmd.Flags().String("project-name", "", "Project name for OpenAPI getTaskInfo")
	cmd.Flags().String("task-name", "", "Task name for OpenAPI getTaskInfo")
	cmd.AddCommand(newTaskResultDownloadCommand())
	return cmd
}

func runTaskResult(cmd *cobra.Command, args []string) error {
	cfg, err := getConfigFromCommand(cmd)
	if err != nil {
		return err
	}
	taskID, _ := cmd.Flags().GetInt64("task-id")
	projectID, _ := cmd.Flags().GetInt64("project-id")
	projectName := strings.TrimSpace(mustGetString(cmd, "project-name"))
	taskName := strings.TrimSpace(mustGetString(cmd, "task-name"))
	if taskID <= 0 && taskName == "" {
		return fmt.Errorf("请提供 --task-id，或提供 --project-id/--project-name 与 --task-name")
	}
	if dryRun {
		if taskID > 0 {
			return outputDryRun(cmd, []dryRunRequest{
				makeDryRunRequest(cfg, http.MethodGet, "/admin-api/scan/task/info", map[string]string{"taskId": strconv.FormatInt(taskID, 10)}, nil, nil),
			})
		}
		body := taskInfoRequestBody(projectID, projectName, taskName)
		return outputDryRun(cmd, []dryRunRequest{
			makeDryRunRequest(cfg, http.MethodPost, "/openapi/v1/getTaskInfo/", nil, body, nil),
		})
	}

	client := NewClient(cfg)
	if taskID > 0 {
		data, err := client.getTaskInfoByID(cmd.Context(), taskID)
		if err != nil {
			return err
		}
		if len(data) == 0 {
			return fmt.Errorf("未找到任务：taskId=%d", taskID)
		}
		return outputOK(cmd, map[string]any{
			"task_id": taskID,
			"result":  data,
		})
	}
	body := taskInfoRequestBody(projectID, projectName, taskName)
	rows, err := client.getTaskInfo(cmd.Context(), body)
	if err != nil {
		return err
	}
	return outputOK(cmd, map[string]any{
		"count":   len(rows),
		"results": rows,
	})
}

func newTaskResultDownloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Export and download a task report",
		RunE:  runTaskResultDownload,
	}
	cmd.Flags().Int64("task-id", 0, "Task ID")
	cmd.Flags().Int64("project-id", 0, "Project ID")
	cmd.Flags().String("out", "", "Local output report path")
	cmd.Flags().Int("report-format", defaultReportFormat, "Report format, 4=json on current CodeInsight versions")
	cmd.Flags().Int("report-specs", defaultReportSpecs, "Report specs, 1=detailed")
	cmd.Flags().Int("wait", defaultReportWait, "Max seconds to wait for export completion")
	cmd.Flags().Int("poll-interval", defaultReportPoll, "Export status polling interval seconds")
	return cmd
}

func runTaskResultDownload(cmd *cobra.Command, args []string) error {
	cfg, err := getConfigFromCommand(cmd)
	if err != nil {
		return err
	}
	taskID, _ := cmd.Flags().GetInt64("task-id")
	projectID, _ := cmd.Flags().GetInt64("project-id")
	outPath := strings.TrimSpace(mustGetString(cmd, "out"))
	if taskID <= 0 {
		return fmt.Errorf("请提供 --task-id")
	}
	if outPath == "" {
		return fmt.Errorf("请提供 --out")
	}
	exportBody := reportExportBody(cmd, taskID, projectID)
	if dryRun {
		return outputDryRun(cmd, []dryRunRequest{
			makeDryRunRequest(cfg, http.MethodPost, "/openapi/v1/task-export/", nil, exportBody, nil),
			makeDryRunRequest(cfg, http.MethodGet, "/openapi/v1/export-status/", map[string]string{"id": "<export-id>"}, nil, nil),
			makeDryRunRequest(cfg, http.MethodGet, "/openapi/v1/download-report/", map[string]string{"id": "<export-id>"}, nil, nil),
		})
	}

	client := NewClient(cfg)
	exportID, err := client.createReportExport(cmd.Context(), exportBody)
	if err != nil {
		return err
	}
	status, err := waitReportExport(cmd, client, exportID)
	if err != nil {
		return err
	}
	data, err := client.downloadReport(cmd.Context(), exportID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), defaultReportDirPerm); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(outPath, data, defaultReportFilePerm); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return outputOK(cmd, map[string]any{
		"task_id":   taskID,
		"export_id": exportID,
		"status":    status,
		"out":       outPath,
		"bytes":     len(data),
	})
}

func buildRepoTaskBody(cmd *cobra.Command, projectName, taskName string, ruleSetID int64, ruleSetName, repoConfigName, repoURL string) (map[string]any, error) {
	body := map[string]any{
		"project_name": projectName,
		"task_name":    taskName,
		"ref_type":     mustGetString(cmd, "ref-type"),
		"ref_name":     mustGetString(cmd, "ref-name"),
	}
	if ruleSetID > 0 {
		body["rule_set_id"] = ruleSetID
	}
	if ruleSetName != "" {
		body["rule_set_name"] = ruleSetName
	}
	if repoConfigName != "" {
		body["repo_config_name"] = repoConfigName
	} else {
		authType := strings.TrimSpace(mustGetString(cmd, "auth-type"))
		direct := map[string]any{
			"repo_type":           mustGetString(cmd, "repo-type"),
			"repo_url":            repoURL,
			"auth_type":           authType,
			"username":            mustGetString(cmd, "username"),
			"password":            mustGetString(cmd, "password"),
			"access_token":        mustGetString(cmd, "access-token"),
			"persist_repo_config": mustGetBool(cmd, "persist-repo-config"),
		}
		if name := strings.TrimSpace(mustGetString(cmd, "persist-repo-config-name")); name != "" {
			direct["repo_config_name"] = name
		}
		if err := validateDirectRepo(direct); err != nil {
			return nil, err
		}
		body["direct_repo"] = direct
	}
	putString(body, "custom_scan_params", mustGetString(cmd, "custom-scan-params"))
	putIntIfChanged(body, "need_command", cmd, "need-command")
	putIntIfChanged(body, "code_metrics", cmd, "code-metrics")
	putString(body, "rule_configuration_set_name", mustGetString(cmd, "rule-configuration-set-name"))
	putString(body, "black_and_white_list_name", mustGetString(cmd, "black-and-white-list-name"))
	putString(body, "compiler_env_path", mustGetString(cmd, "compiler-env-path"))
	putString(body, "build_command", mustGetString(cmd, "build-command"))
	putPositiveInt(body, "memory_limit", cmd, "memory-limit")
	putString(body, "worker_id", mustGetString(cmd, "worker-id"))
	putString(body, "java_repo_config_name", mustGetString(cmd, "java-repo-config-name"))
	putIntIfChanged(body, "incremental_analysis", cmd, "incremental-analysis")
	putString(body, "incremental_base_type", mustGetString(cmd, "incremental-base-type"))
	putString(body, "incremental_base_ref", mustGetString(cmd, "incremental-base-ref"))
	putIntIfChanged(body, "clone_detection", cmd, "clone-detection")
	putPositiveInt(body, "code_clone_granularity", cmd, "code-clone-granularity")
	putIntIfChanged(body, "architecture_analysis", cmd, "architecture-analysis")
	if flagChanged(cmd, "api-asset-inventory") {
		body["api_asset_inventory"] = mustGetBool(cmd, "api-asset-inventory")
	}
	if levels := splitCSV(mustGetString(cmd, "ai-judgment-bug-levels")); len(levels) > 0 {
		body["ai_judgment_bug_levels"] = levels
	}
	if levels := splitCSV(mustGetString(cmd, "deep-analysis-bug-levels")); len(levels) > 0 {
		body["deep_analysis_bug_levels"] = levels
	}
	return body, nil
}

func validateDirectRepo(direct map[string]any) error {
	authType := strings.TrimSpace(fmt.Sprint(direct["auth_type"]))
	username := strings.TrimSpace(fmt.Sprint(direct["username"]))
	password := strings.TrimSpace(fmt.Sprint(direct["password"]))
	accessToken := strings.TrimSpace(fmt.Sprint(direct["access_token"]))
	if authType == "username_password" && (username == "" || password == "") {
		return fmt.Errorf("用户名密码认证必须提供 --username 和 --password")
	}
	if authType == "access_token" && accessToken == "" {
		return fmt.Errorf("Access Token 认证必须提供 --access-token")
	}
	if authType == "anonymous" && (username != "" || password != "" || accessToken != "") {
		return fmt.Errorf("匿名认证不能同时提供凭证字段")
	}
	return nil
}

func parseConfigMode(raw string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "single":
		return repoConfigModeSingle, nil
	case "multi":
		return repoConfigModeMulti, nil
	default:
		return 0, fmt.Errorf("config-mode 仅支持 single 或 multi")
	}
}

func buildRepositoriesFromNames(serverHost string, names []string) []map[string]any {
	repositories := make([]map[string]any, 0, len(names))
	for _, name := range names {
		repoURL := repoURLFromName(serverHost, name)
		repositories = append(repositories, map[string]any{
			"repoName":       filepath.Base(strings.TrimSuffix(name, ".git")),
			"repoFullName":   strings.Trim(strings.TrimSuffix(name, ".git"), "/"),
			"repoUrl":        repoURL,
			"defaultBranch":  "main",
			"visibility":     "",
			"externalRepoId": "",
		})
	}
	return repositories
}

func selectRemoteRepositories(remoteRows []map[string]any, selectors []string) ([]map[string]any, error) {
	selected := make([]map[string]any, 0, len(selectors))
	missing := []string{}
	for _, selector := range selectors {
		var match map[string]any
		for _, row := range remoteRows {
			if selectorMatchesRepo(selector, row) {
				match = row
				break
			}
		}
		if match == nil {
			missing = append(missing, selector)
			continue
		}
		selected = append(selected, map[string]any{
			"repoName":       stringValue(match, "repoName", "repo_name"),
			"repoFullName":   stringValue(match, "repoFullName", "repo_full_name"),
			"repoUrl":        stringValue(match, "repoUrl", "repo_url"),
			"defaultBranch":  stringValue(match, "defaultBranch", "default_branch"),
			"visibility":     stringValue(match, "visibility"),
			"externalRepoId": stringValue(match, "externalRepoId", "external_repo_id"),
		})
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("远端仓库列表中未找到：%s", strings.Join(missing, ", "))
	}
	return selected, nil
}

func selectorMatchesRepo(selector string, row map[string]any) bool {
	selector = strings.TrimSpace(selector)
	candidates := []string{
		stringValue(row, "repoName", "repo_name"),
		stringValue(row, "repoFullName", "repo_full_name"),
		stringValue(row, "repoUrl", "repo_url"),
	}
	for _, candidate := range candidates {
		if candidate == selector {
			return true
		}
	}
	return false
}

func validateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("文件不可访问 %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("路径不是文件：%s", path)
	}
	return nil
}

func fieldsToAny(fields map[string]string) map[string]any {
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		if value != "" {
			out[key] = value
		}
	}
	return out
}

func taskInfoRequestBody(projectID int64, projectName, taskName string) map[string]any {
	body := map[string]any{"task_name": taskName}
	if projectID > 0 {
		body["project_id"] = projectID
	}
	if strings.TrimSpace(projectName) != "" {
		body["project_name"] = strings.TrimSpace(projectName)
	}
	return body
}

func reportExportBody(cmd *cobra.Command, taskID, projectID int64) map[string]any {
	body := map[string]any{
		"scanTaskId":          taskID,
		"reportFormat":        mustGetInt(cmd, "report-format"),
		"reportSpecs":         mustGetInt(cmd, "report-specs"),
		"exportIgnoredData":   1,
		"severityList":        []any{},
		"detectionStatusList": []any{},
		"checkerNameList":     []any{},
		"fileIdList":          []any{},
		"templateId":          0,
		"recodeQuery":         0,
		"sortingFormat":       0,
	}
	if projectID > 0 {
		body["projectId"] = projectID
	}
	return body
}

func waitReportExport(cmd *cobra.Command, client *Client, exportID int64) (map[string]any, error) {
	waitSeconds := mustGetInt(cmd, "wait")
	pollSeconds := mustGetInt(cmd, "poll-interval")
	if waitSeconds <= 0 {
		waitSeconds = defaultReportWait
	}
	if pollSeconds <= 0 {
		pollSeconds = defaultReportPoll
	}
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	var last map[string]any
	for {
		status, err := client.getExportStatus(cmd.Context(), exportID)
		if err != nil {
			return status, err
		}
		last = status
		switch toInt64(firstNonNilMapValue(status, "status")) {
		case 1:
			return status, nil
		case 2:
			return status, fmt.Errorf("报告导出失败：export_id=%d", exportID)
		}
		if time.Now().After(deadline) {
			return last, fmt.Errorf("等待报告导出超时：export_id=%d", exportID)
		}
		select {
		case <-cmd.Context().Done():
			return last, cmd.Context().Err()
		case <-time.After(time.Duration(pollSeconds) * time.Second):
		}
	}
}

func mustGetString(cmd *cobra.Command, name string) string {
	value, _ := cmd.Flags().GetString(name)
	return value
}

func mustGetBool(cmd *cobra.Command, name string) bool {
	value, _ := cmd.Flags().GetBool(name)
	return value
}

func mustGetInt(cmd *cobra.Command, name string) int {
	value, _ := cmd.Flags().GetInt(name)
	return value
}

func intFlagString(cmd *cobra.Command, name string) string {
	value := mustGetInt(cmd, name)
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func putString(body map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		body[key] = value
	}
}

func putPositiveInt(body map[string]any, key string, cmd *cobra.Command, flag string) {
	value := mustGetInt(cmd, flag)
	if value > 0 {
		body[key] = value
	}
}

func putIntIfChanged(body map[string]any, key string, cmd *cobra.Command, flag string) {
	if flagChanged(cmd, flag) {
		body[key] = mustGetInt(cmd, flag)
	}
}
