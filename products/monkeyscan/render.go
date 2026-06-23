package monkeyscan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func printDryRun(cmd *cobra.Command, snapshot *diffSnapshot) {
	fmt.Fprintln(cmd.OutOrStdout(), "MonkeyScan Review Dry Run")
	fmt.Fprintf(cmd.OutOrStdout(), "服务地址: %s\n", normalizedURL(runtimeCfg.URL))
	fmt.Fprintf(cmd.OutOrStdout(), "仓库: %s\n", snapshot.RemoteURL)
	fmt.Fprintf(cmd.OutOrStdout(), "分支: %s\n", firstNonEmpty(snapshot.CurrentBranch, "(detached HEAD)"))
	fmt.Fprintf(cmd.OutOrStdout(), "HEAD: %s\n", snapshot.HeadSHA)
	fmt.Fprintf(cmd.OutOrStdout(), "检查范围: %s\n", snapshot.Scope)
	fmt.Fprintf(cmd.OutOrStdout(), "Diff 字节数: %d\n", len(snapshot.Diff))
	fmt.Fprintf(cmd.OutOrStdout(), "涉及文件数: %d\n", len(snapshot.Files))
	for _, file := range snapshot.Files {
		fmt.Fprintf(cmd.OutOrStdout(), "- %s %s (+%d/-%d)\n", file.Status, file.Path, file.Additions, file.Deletions)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "dry-run 未上传代码、未创建任务、未生成 Review 结果。")
}

func printReviewSummary(cmd *cobra.Command, detail *reviewDetail, reviewPath string) {
	fmt.Fprintln(cmd.OutOrStdout(), "MonkeyScan Review 结果")
	fmt.Fprintf(cmd.OutOrStdout(), "Run ID: %s\n", detail.Run.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "状态: %s\n", firstNonEmpty(detail.Run.Status, detail.Run.TaskGroupStatus))
	fmt.Fprintf(cmd.OutOrStdout(), "仓库: %s\n", detail.Run.RepositoryName)
	fmt.Fprintf(cmd.OutOrStdout(), "发现问题: %d\n", len(detail.Findings))
	fmt.Fprintf(cmd.OutOrStdout(), "结果文件: %s\n", reviewPath)
	fmt.Fprintln(cmd.OutOrStdout(), "提示: .monkeyscan 可能包含代码 diff 和修复建议，请不要提交到仓库。")
}

func writeReviewMarkdown(path string, detail *reviewDetail) error {
	var b strings.Builder
	b.WriteString("# MonkeyScan Review 结果\n\n")
	b.WriteString("## 基础信息\n\n")
	fmt.Fprintf(&b, "- Run ID: %s\n", detail.Run.ID)
	fmt.Fprintf(&b, "- 仓库: %s\n", detail.Run.RepositoryName)
	fmt.Fprintf(&b, "- 状态: %s\n", firstNonEmpty(detail.Run.Status, detail.Run.TaskGroupStatus))
	fmt.Fprintf(&b, "- Base: %s\n", detail.Run.BaseBranch)
	fmt.Fprintf(&b, "- Head: %s %s\n", detail.Run.HeadBranch, detail.Run.HeadSHA)
	fmt.Fprintf(&b, "- 发布时间: %s\n\n", formatTime(detail.Run.UpdatedAt))
	b.WriteString("## 总结\n\n")
	fmt.Fprintf(&b, "- 问题数量: %d\n", len(detail.Findings))
	fmt.Fprintf(&b, "- 评论数量: %d\n", len(detail.Comments))
	if detail.Run.ErrorMessage != "" {
		fmt.Fprintf(&b, "- 错误信息: %s\n", sanitizeErrorMessage(detail.Run.ErrorMessage))
	}
	b.WriteString("\n## 问题列表\n\n")
	if len(detail.Findings) == 0 {
		b.WriteString("未发现问题。\n\n")
	}
	for i, finding := range detail.Findings {
		fmt.Fprintf(&b, "### %d. %s\n\n", i+1, firstNonEmpty(finding.Title, "未命名问题"))
		fmt.Fprintf(&b, "- 严重性: %s\n", finding.Severity)
		fmt.Fprintf(&b, "- 类别: %s\n", finding.Category)
		fmt.Fprintf(&b, "- 置信度: %s\n", finding.Confidence)
		if finding.Location.Path != "" {
			fmt.Fprintf(&b, "- 位置: %s:%d-%d\n", finding.Location.Path, finding.Location.StartLine, finding.Location.EndLine)
		}
		b.WriteString("\n")
		if finding.Description != "" {
			b.WriteString("**描述**\n\n")
			b.WriteString(finding.Description)
			b.WriteString("\n\n")
		}
		if finding.ProblemCode != "" {
			b.WriteString("**问题代码**\n\n```text\n")
			b.WriteString(finding.ProblemCode)
			b.WriteString("\n```\n\n")
		}
		if finding.Recommendation != "" {
			b.WriteString("**修复建议**\n\n")
			b.WriteString(finding.Recommendation)
			b.WriteString("\n\n")
		}
		b.WriteString("**推荐修复 Diff**\n\n```diff\n")
		b.WriteString(firstNonEmpty(finding.SuggestedDiff, finding.RecommendedDiff, finding.FixDiff, finding.SuggestedPatch, "无"))
		b.WriteString("\n```\n\n")
	}
	b.WriteString("## Comments\n\n")
	if len(detail.Comments) == 0 {
		b.WriteString("无。\n")
	}
	for i, comment := range detail.Comments {
		fmt.Fprintf(&b, "### Comment %d\n\n", i+1)
		fmt.Fprintf(&b, "- 类型: %s\n", comment.CommentType)
		fmt.Fprintf(&b, "- 发布状态: %s\n", comment.PublishStatus)
		if comment.ErrorMessage != "" {
			fmt.Fprintf(&b, "- 说明: %s\n", comment.ErrorMessage)
		}
		if comment.Body != "" {
			b.WriteString("\n")
			b.WriteString(comment.Body)
			b.WriteString("\n\n")
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

func writeLocalStatus(runDir string, status localStatus) error {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "status.json"), append(data, '\n'), 0o600)
}

func findLocalStatus(repoRoot, run string) (localStatus, string, error) {
	base := filepath.Join(repoRoot, reviewsDir)
	if run != "" {
		candidate := run
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(base, run)
		}
		if st, err := readLocalStatus(filepath.Join(candidate, "status.json")); err == nil {
			return st, candidate, nil
		}
		if st, dir, err := findStatusByServerRunID(base, run); err == nil {
			return st, dir, nil
		}
		return localStatus{RunID: run, ClientRunID: run}, filepath.Join(base, run), nil
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return localStatus{}, "", fmt.Errorf("未找到本地 Review 状态目录")
	}
	type candidate struct {
		status localStatus
		dir    string
	}
	var candidates []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		st, err := readLocalStatus(filepath.Join(dir, "status.json"))
		if err != nil {
			continue
		}
		if !isTerminalReviewStatus(st.Status, "") {
			candidates = append(candidates, candidate{status: st, dir: dir})
		}
	}
	if len(candidates) == 0 {
		return localStatus{}, "", fmt.Errorf("未找到未完成的 Review，本地可用 --run 指定任务")
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].status.UpdatedAt.After(candidates[j].status.UpdatedAt)
	})
	return candidates[0].status, candidates[0].dir, nil
}

func findStatusByServerRunID(base, runID string) (localStatus, string, error) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return localStatus{}, "", err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		st, err := readLocalStatus(filepath.Join(dir, "status.json"))
		if err == nil && st.RunID == runID {
			return st, dir, nil
		}
	}
	return localStatus{}, "", fmt.Errorf("not found")
}

func readLocalStatus(path string) (localStatus, error) {
	var st localStatus
	data, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(data, &st)
	return st, err
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "未知"
	}
	return t.Format(time.RFC3339)
}
