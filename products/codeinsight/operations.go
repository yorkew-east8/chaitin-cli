package codeinsight

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) findProjectByName(ctx context.Context, name string) (*projectRecord, error) {
	query := url.Values{}
	query.Set("projectName", name)
	var items []map[string]any
	if err := c.DoJSON(ctx, http.MethodGet, "/admin-api/scan/project/simple-project-list", query, nil, &items); err != nil {
		return nil, err
	}

	var matches []projectRecord
	for _, item := range items {
		itemName := stringValue(item, "scanProjectName", "projectName", "name")
		if itemName != name {
			continue
		}
		matches = append(matches, projectRecord{
			ID:   int64Value(item, "id", "projectId", "project_id"),
			Name: itemName,
			Raw:  item,
		})
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("存在多个同名项目 %q，请先在平台清理或重命名", name)
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return &matches[0], nil
}

func (c *Client) createProject(ctx context.Context, name string, languageID int64) (int64, error) {
	body := map[string]any{
		"projectName":    name,
		"languageId":     languageID,
		"projectGroupId": 0,
		"type":           0,
		"members":        []any{},
	}
	var projectID any
	if err := c.DoJSON(ctx, http.MethodPost, "/admin-api/scan/project", nil, body, &projectID); err != nil {
		return 0, err
	}
	if id := toInt64(projectID); id > 0 {
		return id, nil
	}
	return 0, fmt.Errorf("项目创建失败：平台未返回项目 ID")
}

func (c *Client) createOnlineScan(ctx context.Context, fields map[string]string, files map[string]string) (int64, error) {
	var taskID any
	if err := c.DoMultipart(ctx, "/openapi/v1/createOnlineScan", fields, files, &taskID); err != nil {
		return 0, err
	}
	if id := toInt64(taskID); id > 0 {
		return id, nil
	}
	return 0, fmt.Errorf("任务创建失败：平台未返回任务 ID")
}

func (c *Client) listRepoConfigs(ctx context.Context) ([]repoConfigRecord, error) {
	body := map[string]any{
		"pageNo":   1,
		"pageSize": defaultRepoConfigLimit,
	}
	var page map[string]any
	if err := c.DoJSON(ctx, http.MethodPost, "/admin-api/scan/code/list", nil, body, &page); err != nil {
		return nil, err
	}
	rows := rowsFromPage(page)
	configs := make([]repoConfigRecord, 0, len(rows))
	for _, row := range rows {
		repoType := int(toInt64(firstNonNilMapValue(row, "repoType", "repo_type")))
		configs = append(configs, repoConfigRecord{
			ID:         int64Value(row, "id"),
			Name:       stringValue(row, "name"),
			RepoType:   repoType,
			ServerHost: stringValue(row, "serverHost", "server_host"),
			Raw:        row,
		})
	}
	return configs, nil
}

func rowsFromPage(page map[string]any) []map[string]any {
	for _, key := range []string{"list", "records", "items"} {
		if rows, ok := page[key].([]any); ok {
			return mapRows(rows)
		}
	}
	if rows, ok := page["data"].([]any); ok {
		return mapRows(rows)
	}
	return nil
}

func mapRows(rows []any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func findRepoConfigByName(configs []repoConfigRecord, name string) *repoConfigRecord {
	for _, cfg := range configs {
		if cfg.Name == name {
			copy := cfg
			return &copy
		}
	}
	return nil
}

func (c *Client) checkGitConfiguration(ctx context.Context, payload map[string]any) error {
	var result any
	return c.DoJSON(ctx, http.MethodPost, "/admin-api/scan/code/check-git-configuration", nil, payload, &result)
}

func (c *Client) listRemoteRepositories(ctx context.Context, payload map[string]any) ([]map[string]any, error) {
	var rows []map[string]any
	if err := c.DoJSON(ctx, http.MethodPost, "/admin-api/scan/code/list-remote-repositories", nil, payload, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *Client) saveRepoConfig(ctx context.Context, payload []map[string]any) error {
	var saved any
	return c.DoJSON(ctx, http.MethodPost, "/admin-api/scan/code/save", nil, payload, &saved)
}

func (c *Client) createRepoOnlineScan(ctx context.Context, body map[string]any) (map[string]any, error) {
	var data map[string]any
	if err := c.DoJSON(ctx, http.MethodPost, "/openapi/v1/createRepoOnlineScan", nil, body, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Client) getTaskInfo(ctx context.Context, body map[string]any) ([]map[string]any, error) {
	var rows []map[string]any
	if err := c.DoJSON(ctx, http.MethodPost, "/openapi/v1/getTaskInfo/", nil, body, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (c *Client) getTaskInfoByID(ctx context.Context, taskID int64) (map[string]any, error) {
	query := url.Values{}
	query.Set("taskId", strconv.FormatInt(taskID, 10))
	var data map[string]any
	if err := c.DoJSON(ctx, http.MethodGet, "/admin-api/scan/task/info", query, nil, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Client) createReportExport(ctx context.Context, body map[string]any) (int64, error) {
	var exportID any
	if err := c.DoJSON(ctx, http.MethodPost, "/openapi/v1/task-export/", nil, body, &exportID); err != nil {
		return 0, err
	}
	if id := toInt64(exportID); id > 0 {
		return id, nil
	}
	return 0, fmt.Errorf("报告导出失败：平台未返回导出 ID")
}

func (c *Client) getExportStatus(ctx context.Context, exportID int64) (map[string]any, error) {
	query := url.Values{}
	query.Set("id", strconv.FormatInt(exportID, 10))
	var data map[string]any
	if err := c.DoJSON(ctx, http.MethodGet, "/openapi/v1/export-status/", query, nil, &data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *Client) downloadReport(ctx context.Context, exportID int64) ([]byte, error) {
	query := url.Values{}
	query.Set("id", strconv.FormatInt(exportID, 10))
	data, _, err := c.Download(ctx, "/openapi/v1/download-report/", query)
	return data, err
}
