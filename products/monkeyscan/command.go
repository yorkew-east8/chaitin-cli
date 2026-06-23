package monkeyscan

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chaitin/chaitin-cli/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	runtimeCfg        Config
	runtimeConfigPath string
	dryRun            bool
	readSecret        = readSecretFromTerminal
	nowFunc           = time.Now
	pollInterval      = 2 * time.Second
	pollTimeout       = 10 * time.Minute
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monkeyscan",
		Short: "MonkeyScan local review",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			applyRuntimeConfig()
		},
	}
	cmd.AddCommand(newAuthCommand())
	cmd.AddCommand(newReviewCommand())
	return cmd
}

func ApplyRuntimeConfig(cmd *cobra.Command, cfg config.Raw, configPath string, isDryRun bool) {
	productCfg, err := config.DecodeProduct[Config](cfg, productName)
	if err == nil {
		runtimeCfg = productCfg
	}
	runtimeConfigPath = configPath
	dryRun = isDryRun
}

func applyRuntimeConfig() {
	runtimeCfg.URL = normalizedURL(runtimeCfg.URL)
}

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage MonkeyScan API key"}
	cmd.AddCommand(newAuthSetKeyCommand())
	cmd.AddCommand(newAuthStatusCommand())
	cmd.AddCommand(newAuthClearCommand())
	return cmd
}

func newAuthSetKeyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set-key",
		Short: "Save MonkeyScan API key locally",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtimeConfigPath == "" {
				return fmt.Errorf("config path is empty")
			}
			fmt.Fprint(cmd.OutOrStdout(), "请输入 MonkeyScan CLI API Key: ")
			key, err := readSecret()
			fmt.Fprintln(cmd.OutOrStdout())
			if err != nil {
				return err
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return fmt.Errorf("key 不能为空")
			}
			newCfg := runtimeCfg
			newCfg.URL = normalizedURL(newCfg.URL)
			newCfg.Key = key
			if err := config.SetProduct(runtimeConfigPath, productName, newCfg); err != nil {
				return err
			}
			runtimeCfg = newCfg
			fmt.Fprintf(cmd.OutOrStdout(), "已保存 MonkeyScan CLI API Key 到 %s\n", runtimeConfigPath)
			return nil
		},
	}
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show MonkeyScan authorization status",
		RunE: func(cmd *cobra.Command, args []string) error {
			key, source := resolveKey()
			fmt.Fprintln(cmd.OutOrStdout(), "MonkeyScan 授权状态")
			fmt.Fprintf(cmd.OutOrStdout(), "服务地址: %s\n", normalizedURL(runtimeCfg.URL))
			if key == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Key 状态: 未配置")
				fmt.Fprintf(cmd.OutOrStdout(), "Key 来源: %s\n", keySourceNone)
				fmt.Fprintln(cmd.OutOrStdout(), "请先运行 monkeyscan auth set-key 或设置 MONKEYSCAN_KEY")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Key 状态: 已配置")
			fmt.Fprintf(cmd.OutOrStdout(), "Key 来源: %s\n", source)
			if dryRun {
				return nil
			}
			client := newClient(normalizedURL(runtimeCfg.URL), key)
			status, err := client.Status(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "账号: %s", status.Account.Name)
			if status.Account.Email != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " <%s>", status.Account.Email)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintf(cmd.OutOrStdout(), "GitHub 绑定: %s\n", yesNo(status.GitHubBound))
			if status.GitHubAccount != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "GitHub 账号: %s\n", status.GitHubAccount)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "同步仓库数: %d\n", status.SyncedRepositoryCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Review 就绪: %s\n", yesNo(status.ReviewReady))
			for _, msg := range status.Messages {
				fmt.Fprintf(cmd.OutOrStdout(), "提示: %s\n", msg)
			}
			return nil
		},
	}
}

func newAuthClearCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Clear locally saved MonkeyScan API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtimeConfigPath == "" {
				return fmt.Errorf("config path is empty")
			}
			newCfg := runtimeCfg
			newCfg.URL = normalizedURL(newCfg.URL)
			newCfg.Key = ""
			if err := config.SetProduct(runtimeConfigPath, productName, newCfg); err != nil {
				return err
			}
			runtimeCfg = newCfg
			fmt.Fprintf(cmd.OutOrStdout(), "已清除本机保存的 MonkeyScan CLI API Key: %s\n", runtimeConfigPath)
			if os.Getenv(envKeyName) != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "环境变量 MONKEYSCAN_KEY 仍会继续生效")
			}
			return nil
		},
	}
}

func newReviewCommand() *cobra.Command {
	opts := reviewScope{}
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Run MonkeyScan local diff review",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Type, "type", "", "Review scope (uncommitted|staged|committed)")
	cmd.Flags().BoolVar(&opts.All, "all", false, "Review unstaged and staged changes")
	cmd.Flags().StringVar(&opts.Base, "base", "", "Base branch for committed review")
	cmd.Flags().StringVar(&opts.BaseCommit, "base-commit", "", "Base commit for committed review")
	cmd.AddCommand(newReviewStatusCommand())
	return cmd
}

func newReviewStatusCommand() *cobra.Command {
	var run string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Fetch a submitted MonkeyScan review result",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReviewStatus(cmd, run)
		},
	}
	cmd.Flags().StringVar(&run, "run", "", "Client run id, server run id, or local run directory")
	return cmd
}

func runReview(cmd *cobra.Command, opts reviewScope) error {
	if err := validateReviewScope(opts); err != nil {
		return err
	}
	snapshot, err := collectDiff(cmd.Context(), opts)
	if err != nil {
		return err
	}
	if strings.TrimSpace(snapshot.Diff) == "" {
		fmt.Fprintln(cmd.OutOrStdout(), "指定范围内没有可 Review 的代码改动，未发起 Review。")
		return nil
	}
	if dryRun {
		printDryRun(cmd, snapshot)
		return nil
	}
	key, _ := resolveKey()
	if key == "" {
		return fmt.Errorf("未配置 MonkeyScan CLI API Key，请先运行 monkeyscan auth set-key 或设置 MONKEYSCAN_KEY")
	}
	req := buildReviewRequest(cmd, snapshot, "")
	if size, err := encodedSize(req); err == nil && size > maxReviewBodySize {
		return fmt.Errorf("Review 上传内容过大：%d bytes，超过服务端限制 %d bytes", size, maxReviewBodySize)
	}
	client := newClient(normalizedURL(runtimeCfg.URL), key)
	status, err := client.Status(cmd.Context())
	if err != nil {
		return err
	}
	if !status.ReviewReady {
		for _, msg := range status.Messages {
			fmt.Fprintf(cmd.OutOrStdout(), "提示: %s\n", msg)
		}
		return fmt.Errorf("当前账号尚未满足差分 Review 前置条件")
	}
	clientRunID := newClientRunID()
	req = buildReviewRequest(cmd, snapshot, clientRunID)
	runDir := filepath.Join(snapshot.RepoRoot, reviewsDir, clientRunID)
	reviewPath := filepath.Join(runDir, "review.md")
	resp, err := client.CreateDiffReview(cmd.Context(), req)
	if err != nil {
		return err
	}
	local := localStatus{
		ClientRunID: clientRunID,
		RunID:       resp.RunID,
		TaskGroupID: resp.TaskGroupID,
		Scope:       snapshot.Scope,
		Status:      resp.Status,
		BaseBranch:  snapshot.BaseBranch,
		BaseSHA:     snapshot.BaseSHA,
		HeadBranch:  snapshot.CurrentBranch,
		HeadSHA:     snapshot.HeadSHA,
		ReviewPath:  reviewPath,
		CreatedAt:   nowFunc(),
		UpdatedAt:   nowFunc(),
	}
	if err := writeLocalStatus(runDir, local); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Review 任务已提交: %s\n", resp.RunID)
	detail, err := waitReviewResult(cmd.Context(), client, resp.RunID)
	if err != nil {
		local.Status = "interrupted"
		local.ErrorMessage = sanitizeErrorMessage(err.Error())
		local.UpdatedAt = nowFunc()
		_ = writeLocalStatus(runDir, local)
		return fmt.Errorf("任务已提交但结果获取中断，请稍后运行 monkeyscan review status --run %s: %w", clientRunID, err)
	}
	return finishReview(cmd, runDir, reviewPath, local, detail)
}

func runReviewStatus(cmd *cobra.Command, run string) error {
	key, _ := resolveKey()
	if key == "" {
		return fmt.Errorf("未配置 MonkeyScan CLI API Key，请先运行 monkeyscan auth set-key 或设置 MONKEYSCAN_KEY")
	}
	repoRoot, _ := gitOutput(cmd.Context(), "", "rev-parse", "--show-toplevel")
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}
	local, runDir, err := findLocalStatus(repoRoot, run)
	if err != nil {
		return err
	}
	runID := local.RunID
	if runID == "" && run != "" {
		runID = run
	}
	if runID == "" {
		return fmt.Errorf("未找到可查询的 Review run id")
	}
	client := newClient(normalizedURL(runtimeCfg.URL), key)
	detail, err := client.ReviewDetail(cmd.Context(), runID)
	if err != nil {
		return err
	}
	reviewPath := local.ReviewPath
	if reviewPath == "" {
		reviewPath = filepath.Join(runDir, "review.md")
	}
	if runDir == "" {
		runDir = filepath.Join(repoRoot, reviewsDir, firstNonEmpty(local.ClientRunID, runID))
	}
	return finishReview(cmd, runDir, reviewPath, local, detail)
}

func finishReview(cmd *cobra.Command, runDir, reviewPath string, local localStatus, detail *reviewDetail) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	if err := writeReviewMarkdown(reviewPath, detail); err != nil {
		return err
	}
	local.Status = firstNonEmpty(detail.Run.Status, detail.Run.TaskGroupStatus)
	local.RunID = firstNonEmpty(local.RunID, detail.Run.ID)
	local.TaskGroupID = firstNonEmpty(local.TaskGroupID, detail.Run.TaskGroupID)
	local.ReviewPath = reviewPath
	local.ErrorMessage = sanitizeErrorMessage(detail.Run.ErrorMessage)
	local.UpdatedAt = nowFunc()
	if err := writeLocalStatus(runDir, local); err != nil {
		return err
	}
	printReviewSummary(cmd, detail, reviewPath)
	if isFailedReviewStatus(detail.Run.Status, detail.Run.TaskGroupStatus) {
		return fmt.Errorf("Review 任务失败，结果已写入 %s", reviewPath)
	}
	return nil
}

func validateReviewScope(opts reviewScope) error {
	opts.Type = strings.TrimSpace(opts.Type)
	if opts.All && opts.Type != "" {
		return fmt.Errorf("--all 不能和 --type 同时使用")
	}
	if !opts.All && opts.Type == "" {
		return fmt.Errorf("请通过 --type 或 --all 明确指定检查范围")
	}
	if opts.All {
		if opts.Base != "" || opts.BaseCommit != "" {
			return fmt.Errorf("--all 不能和 --base 或 --base-commit 同时使用")
		}
		return nil
	}
	switch opts.Type {
	case "uncommitted", "staged":
		if opts.Base != "" || opts.BaseCommit != "" {
			return fmt.Errorf("--base 和 --base-commit 仅支持 --type committed")
		}
	case "committed":
		if (opts.Base == "") == (opts.BaseCommit == "") {
			return fmt.Errorf("--type committed 必须且只能指定 --base 或 --base-commit")
		}
	default:
		return fmt.Errorf("--type 仅支持 uncommitted、staged 或 committed")
	}
	return nil
}

func buildReviewRequest(cmd *cobra.Command, snapshot *diffSnapshot, clientRunID string) diffReviewRequest {
	return diffReviewRequest{
		ClientRunID:  clientRunID,
		Command:      safeCommandPath(cmd),
		Scope:        snapshot.Scope,
		GitRemoteURL: snapshot.RemoteURL,
		BaseBranch:   snapshot.BaseBranch,
		BaseSHA:      snapshot.BaseSHA,
		HeadBranch:   snapshot.CurrentBranch,
		HeadSHA:      snapshot.HeadSHA,
		Diff:         snapshot.Diff,
		Files:        snapshot.Files,
	}
}

func safeCommandPath(cmd *cobra.Command) string {
	if cmd != nil {
		if path := strings.TrimSpace(cmd.CommandPath()); path != "" {
			return path
		}
	}
	if len(os.Args) == 0 {
		return ""
	}
	return filepath.Base(os.Args[0])
}

func resolveKey() (string, keySource) {
	if key := strings.TrimSpace(os.Getenv(envKeyName)); key != "" {
		return key, keySourceEnv
	}
	if key := strings.TrimSpace(runtimeCfg.Key); key != "" {
		return key, keySourceConfig
	}
	return "", keySourceNone
}

func normalizedURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultURL
	}
	return strings.TrimRight(value, "/")
}

func readSecretFromTerminal() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		data, err := term.ReadPassword(int(os.Stdin.Fd()))
		return string(data), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	return line, err
}

func newClientRunID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("cli-%d", nowFunc().UnixNano())
	}
	return fmt.Sprintf("cli-%s", hex.EncodeToString(b[:]))
}

func encodedSize(v any) (int, error) {
	data, err := json.Marshal(v)
	return len(data), err
}

func yesNo(v bool) string {
	if v {
		return "是"
	}
	return "否"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var (
	apiKeyPattern = regexp.MustCompile(`(?i)(msk_(?:live|test)_[A-Za-z0-9_-]+|bearer\s+[A-Za-z0-9._~+/=-]+|api[_-]?key["'\s:=]+[A-Za-z0-9._~+/=-]+|token["'\s:=]+[A-Za-z0-9._~+/=-]+)`)
	urlPattern    = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

func sanitizeErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	message = apiKeyPattern.ReplaceAllString(message, "[redacted-secret]")
	message = urlPattern.ReplaceAllString(message, "[redacted-url]")
	message = strings.Join(strings.Fields(message), " ")
	const maxLen = 500
	if len(message) > maxLen {
		message = message[:maxLen] + "..."
	}
	return message
}
