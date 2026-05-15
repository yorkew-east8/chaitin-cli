package version

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// versionCacheEntry represents a cached server version lookup.
type versionCacheEntry struct {
	URL       string `json:"url"`
	Version   string `json:"version"`
	CheckedAt string `json:"checked_at"`
}

// apiResponse represents the API response envelope from SafeLine.
type apiResponse struct {
	Err  interface{} `json:"err"`
	Data interface{} `json:"data"`
}

// GetServerVersionFromAPI calls the SafeLine server to determine its version.
// It sends a GET request to {baseURL}/api/ServerControlledConfigAPI and
// extracts the "version" field from the response data.
func GetServerVersionFromAPI(baseURL string, httpClient *http.Client) (string, error) {
	url := baseURL + "/api/ServerControlledConfigAPI"

	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for API-level error
	if apiResp.Err != nil {
		errStr, ok := apiResp.Err.(string)
		if !ok || errStr != "" {
			if ok {
				return "", fmt.Errorf("API error: %s", errStr)
			}
			return "", fmt.Errorf("API returned error: %v", apiResp.Err)
		}
	}

	// Extract version from data
	data, ok := apiResp.Data.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected data type in response")
	}

	versionVal, ok := data["version"]
	if !ok {
		return "", fmt.Errorf("version field not found in response data")
	}

	version, ok := versionVal.(string)
	if !ok {
		return "", fmt.Errorf("version field is not a string")
	}

	return version, nil
}

// CacheVersion writes a version cache entry to the specified path.
// It creates parent directories as needed.
func CacheVersion(cachePath, serverURL, version string) error {
	entry := versionCacheEntry{
		URL:       serverURL,
		Version:   version,
		CheckedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// LoadCachedVersion reads a cached version entry from the specified path.
// Returns an error if the file doesn't exist, is corrupt, or the URL doesn't match.
func LoadCachedVersion(cachePath, serverURL string) (*versionCacheEntry, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if _, ok := err.(*fs.PathError); ok {
			return nil, fmt.Errorf("cache file not found: %w", err)
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var entry versionCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	if entry.URL != serverURL {
		return nil, fmt.Errorf("cached URL %q does not match requested URL %q", entry.URL, serverURL)
	}

	return &entry, nil
}

// DefaultCachePath returns the default path for the SafeLine version cache file.
func DefaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home cannot be determined
		return "safeline-version"
	}
	return filepath.Join(home, ".chaitin-cli", "cache", "safeline-version")
}
