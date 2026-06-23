package monkeyscan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type client struct {
	baseURL    string
	key        string
	httpClient *http.Client
}

func newClient(baseURL, key string) *client {
	return &client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     strings.TrimSpace(key),
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *client) Status(ctx context.Context) (*statusResponse, error) {
	var out statusResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/cli/status", nil, &out); err != nil {
		return nil, err
	}
	if out.Ready && !out.ReviewReady {
		out.ReviewReady = true
	}
	return &out, nil
}

func (c *client) CreateDiffReview(ctx context.Context, req diffReviewRequest) (*diffReviewResponse, error) {
	var out diffReviewResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/cli/reviews/diff", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) ReviewDetail(ctx context.Context, runID string) (*reviewDetail, error) {
	var out reviewDetail
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/cli/reviews/"+url.PathEscape(runID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("编码请求失败: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("请求 MonkeyScan 服务失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("MonkeyScan API 返回错误 (%d): %s", resp.StatusCode, responseErrorMessage(data))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

func responseErrorMessage(data []byte) string {
	var payload struct {
		Error   any `json:"error"`
		Message any `json:"message"`
		Msg     any `json:"msg"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		for _, v := range []any{payload.Error, payload.Message, payload.Msg} {
			if text := strings.TrimSpace(fmt.Sprint(v)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return strings.TrimSpace(string(data))
}

func waitReviewResult(ctx context.Context, client *client, runID string) (*reviewDetail, error) {
	deadline := time.NewTimer(pollTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		detail, err := client.ReviewDetail(ctx, runID)
		if err != nil {
			return nil, err
		}
		if isTerminalReviewStatus(detail.Run.Status, detail.Run.TaskGroupStatus) {
			return detail, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, fmt.Errorf("等待 Review 结果超时")
		case <-ticker.C:
		}
	}
}

func isTerminalReviewStatus(runStatus, groupStatus string) bool {
	return isSuccessfulReviewStatus(runStatus, groupStatus) || isFailedReviewStatus(runStatus, groupStatus)
}

func isSuccessfulReviewStatus(runStatus, groupStatus string) bool {
	if strings.EqualFold(runStatus, "completed") || strings.EqualFold(runStatus, "success") || strings.EqualFold(groupStatus, "success") {
		return true
	}
	return false
}

func isFailedReviewStatus(runStatus, groupStatus string) bool {
	switch strings.ToLower(runStatus) {
	case "failed", "skipped":
		return true
	}
	if strings.EqualFold(groupStatus, "failed") {
		return true
	}
	return false
}
