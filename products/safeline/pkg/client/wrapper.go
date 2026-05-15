package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Envelope is the standard Skyview API response wrapper.
type Envelope struct {
	Err  *string         `json:"err"`
	Data json.RawMessage `json:"data"`
	Msg  json.RawMessage `json:"msg,omitempty"`
}

// Message represents an optional message in the API response.
type Message struct {
	Level string `json:"level,omitempty"`
	Text  string `json:"text,omitempty"`
}

// GetMsg parses the msg field which can be either a string or a Message object.
func (e *Envelope) GetMsg() string {
	if e.Msg == nil {
		return ""
	}
	// Try to parse as string first
	var msgStr string
	if err := json.Unmarshal(e.Msg, &msgStr); err == nil {
		return msgStr
	}
	// Try to parse as Message object
	var msgObj Message
	if err := json.Unmarshal(e.Msg, &msgObj); err == nil {
		return msgObj.Text
	}
	return string(e.Msg)
}

// IsWarning returns true if the msg is a Message object with level "warning".
func (e *Envelope) IsWarning() bool {
	if e.Msg == nil {
		return false
	}
	var msgObj Message
	if err := json.Unmarshal(e.Msg, &msgObj); err == nil {
		return msgObj.Level == "warning"
	}
	return false
}

// GetWarningText returns the warning text if msg is a warning Message.
func (e *Envelope) GetWarningText() string {
	if e.Msg == nil {
		return ""
	}
	var msgObj Message
	if err := json.Unmarshal(e.Msg, &msgObj); err == nil {
		if msgObj.Level == "warning" {
			return msgObj.Text
		}
	}
	return ""
}

// Client wraps HTTP communication with the Skyview API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates a new Skyview API client.
func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
	}
}

// Do performs an HTTP request and unwraps the Skyview response envelope.
// query supports both single values (string) and multiple values ([]string).
func (c *Client) Do(method, path string, body io.Reader, query map[string]string) (*Envelope, error) {
	return c.DoMulti(method, path, body, query, nil)
}

// DoMulti performs an HTTP request with support for multi-value query parameters.
// singleQuery is for single-value params, multiQuery is for multi-value params (like tail_sort).
func (c *Client) DoMulti(method, path string, body io.Reader, singleQuery map[string]string, multiQuery map[string][]string) (*Envelope, error) {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	q := u.Query()
	for k, v := range singleQuery {
		q.Set(k, v)
	}
	for k, vs := range multiQuery {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var envelope Envelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if envelope.Err != nil && *envelope.Err != "" {
		// Get error message from Msg field (can be string or object)
		errMsg := envelope.GetMsg()
		if errMsg == "" {
			errMsg = string(envelope.Data)
		}
		return &envelope, fmt.Errorf("API error [%s]: %s", *envelope.Err, errMsg)
	}

	return &envelope, nil
}

// DoPaginated auto-paginates through all results.
func (c *Client) DoPaginated(path string, query map[string]string) ([]json.RawMessage, error) {
	var allItems []json.RawMessage

	if query == nil {
		query = make(map[string]string)
	}

	for {
		envelope, err := c.Do("GET", path, nil, query)
		if err != nil {
			return nil, err
		}

		// Try to extract paginated items
		var paged struct {
			Items json.RawMessage `json:"items"`
			Total *int            `json:"total"`
		}
		if err := json.Unmarshal(envelope.Data, &paged); err != nil || paged.Items == nil {
			return []json.RawMessage{envelope.Data}, nil
		}

		var items []json.RawMessage
		if err := json.Unmarshal(paged.Items, &items); err != nil {
			return []json.RawMessage{envelope.Data}, nil
		}

		allItems = append(allItems, items...)

		if paged.Total == nil {
			break
		}

		count := 100
		if c, ok := query["count"]; ok {
			if n, err := strconv.Atoi(c); err == nil {
				count = n
			}
		}

		offset := 0
		if o, ok := query["offset"]; ok {
			if n, err := strconv.Atoi(o); err == nil {
				offset = n
			}
		}

		offset += count
		if offset >= *paged.Total {
			break
		}
		query["offset"] = strconv.Itoa(offset)
	}

	return allItems, nil
}

// paramErrorPatterns are substrings that indicate an API error caused by
// an unsupported or unrecognised query parameter (typically from newer
// client versions talking to older server versions).
var paramErrorPatterns = []string{
	"[invalid_param",
	"[invalid_filter_key",
	"unknown parameter",
	"unexpected parameter",
}

// IsParamError reports whether err is caused by an unsupported parameter.
// It checks the error message for patterns returned by the SafeLine API
// when a query parameter is not recognised by the server version.
func IsParamError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, p := range paramErrorPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// DoWithFallback performs an HTTP request and retries with fallback query
// parameters when the server returns a parameter validation error.
// This supports compatibility with older server versions that reject
// newer query parameters.
func (c *Client) DoWithFallback(method, path string, body io.Reader, query, fallbackQuery map[string]string) (*Envelope, error) {
	env, err := c.Do(method, path, body, query)
	if err == nil {
		return env, nil
	}
	if IsParamError(err) && fallbackQuery != nil {
		if fbEnv, fbErr := c.Do(method, path, body, fallbackQuery); fbErr == nil {
			return fbEnv, nil
		}
	}
	return env, err
}

// DoMultiWithFallback performs an HTTP request with multi-value query
// parameter support and retries with fallback parameters when the server
// returns a parameter validation error.
func (c *Client) DoMultiWithFallback(method, path string, body io.Reader,
	singleQuery map[string]string, multiQuery map[string][]string,
	fallbackSingleQuery map[string]string, fallbackMultiQuery map[string][]string,
) (*Envelope, error) {
	env, err := c.DoMulti(method, path, body, singleQuery, multiQuery)
	if err == nil {
		return env, nil
	}
	if IsParamError(err) && (fallbackSingleQuery != nil || fallbackMultiQuery != nil) {
		if fbEnv, fbErr := c.DoMulti(method, path, body, fallbackSingleQuery, fallbackMultiQuery); fbErr == nil {
			return fbEnv, nil
		}
	}
	return env, err
}
