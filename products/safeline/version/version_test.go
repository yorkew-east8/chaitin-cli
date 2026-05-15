package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetServerVersionFromAPI(t *testing.T) {
	t.Run("successful response", func(t *testing.T) {
		expectedVersion := "23.01.014"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/ServerControlledConfigAPI" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			resp := map[string]interface{}{
				"err": nil,
				"data": map[string]interface{}{
					"version": expectedVersion,
					"other":   "ignored",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		got, err := GetServerVersionFromAPI(server.URL, server.Client())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != expectedVersion {
			t.Errorf("got %q, want %q", got, expectedVersion)
		}
	})

	t.Run("server returns error in envelope", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"err":  "some error",
				"data": nil,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		_, err := GetServerVersionFromAPI(server.URL, server.Client())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("server returns non-200 status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		_, err := GetServerVersionFromAPI(server.URL, server.Client())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing version in data", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"err":  nil,
				"data": map[string]interface{}{"other": "value"},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		_, err := GetServerVersionFromAPI(server.URL, server.Client())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, "not json")
		}))
		defer server.Close()

		_, err := GetServerVersionFromAPI(server.URL, server.Client())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestCacheVersion(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "version-cache")

	err := CacheVersion(cachePath, "https://safeline.example.com", "23.01.014")
	if err != nil {
		t.Fatalf("CacheVersion error: %v", err)
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("failed to read cache file: %v", err)
	}

	var entry versionCacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("failed to unmarshal cache: %v", err)
	}

	if entry.URL != "https://safeline.example.com" {
		t.Errorf("URL = %q, want %q", entry.URL, "https://safeline.example.com")
	}
	if entry.Version != "23.01.014" {
		t.Errorf("Version = %q, want %q", entry.Version, "23.01.014")
	}
	if entry.CheckedAt == "" {
		t.Error("CheckedAt is empty")
	}
	if _, parseErr := time.Parse(time.RFC3339, entry.CheckedAt); parseErr != nil {
		t.Errorf("CheckedAt is not valid RFC3339: %v", parseErr)
	}
}

func TestCacheVersionCreatesParentDir(t *testing.T) {
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "subdir", "nested", "version-cache")

	err := CacheVersion(cachePath, "https://safeline.example.com", "25.03.001")
	if err != nil {
		t.Fatalf("CacheVersion error: %v", err)
	}

	if _, statErr := os.Stat(cachePath); statErr != nil {
		t.Fatalf("cache file not created: %v", statErr)
	}
}

func TestLoadCachedVersion(t *testing.T) {
	t.Run("valid cache with matching URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "version-cache")
		serverURL := "https://safeline.example.com"
		version := "23.01.014"

		if err := CacheVersion(cachePath, serverURL, version); err != nil {
			t.Fatalf("CacheVersion error: %v", err)
		}

		entry, err := LoadCachedVersion(cachePath, serverURL)
		if err != nil {
			t.Fatalf("LoadCachedVersion error: %v", err)
		}
		if entry.Version != version {
			t.Errorf("Version = %q, want %q", entry.Version, version)
		}
		if entry.URL != serverURL {
			t.Errorf("URL = %q, want %q", entry.URL, serverURL)
		}
	})

	t.Run("cache with non-matching URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "version-cache")

		if err := CacheVersion(cachePath, "https://old.example.com", "23.01.014"); err != nil {
			t.Fatalf("CacheVersion error: %v", err)
		}

		_, err := LoadCachedVersion(cachePath, "https://new.example.com")
		if err == nil {
			t.Fatal("expected error for non-matching URL, got nil")
		}
	})

	t.Run("missing cache file", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "nonexistent")

		_, err := LoadCachedVersion(cachePath, "https://safeline.example.com")
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("corrupt cache file", func(t *testing.T) {
		tmpDir := t.TempDir()
		cachePath := filepath.Join(tmpDir, "version-cache")

		if err := os.WriteFile(cachePath, []byte("not json"), 0o644); err != nil {
			t.Fatalf("write error: %v", err)
		}

		_, err := LoadCachedVersion(cachePath, "https://safeline.example.com")
		if err == nil {
			t.Fatal("expected error for corrupt file, got nil")
		}
	})
}

func TestDefaultCachePath(t *testing.T) {
	path := DefaultCachePath()
	if path == "" {
		t.Fatal("DefaultCachePath returned empty string")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	expected := filepath.Join(home, ".chaitin-cli", "cache", "safeline-version")
	if path != expected {
		t.Errorf("DefaultCachePath() = %q, want %q", path, expected)
	}
}
