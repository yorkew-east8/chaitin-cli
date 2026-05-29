package site

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/chaitin/chaitin-cli/products/safeline/pkg/client"
)

const fileStorageObjectMissing = "FileStorage: Object does not exist"

type siteOperationClient interface {
	Do(method, path string, body io.Reader, query map[string]string) (*client.Envelope, error)
}

func recoverCreateFileStorageError(c siteOperationClient, endpoint string, payload map[string]any, env *client.Envelope, originalErr error, warnings []string) (*client.Envelope, []string, bool, error) {
	if !isFileStorageObjectMissing(env, originalErr) {
		return nil, warnings, false, originalErr
	}
	name, _ := payload["name"].(string)
	if name == "" {
		return nil, warnings, false, originalErr
	}
	sites, err := listSitesForRecovery(c, endpoint)
	if err != nil {
		return nil, warnings, false, originalErr
	}
	for _, site := range sites {
		if site.Name == name {
			data, err := json.Marshal(site.Raw)
			if err != nil {
				return nil, warnings, false, originalErr
			}
			warnings = append(warnings, fileStorageRecoveryWarning("create", "site was created"))
			return &client.Envelope{Data: data}, warnings, true, nil
		}
	}
	return nil, warnings, false, originalErr
}

func recoverDeleteFileStorageError(c siteOperationClient, endpoint string, id int, env *client.Envelope, originalErr error, warnings []string) (*client.Envelope, []string, bool, error) {
	if !isFileStorageObjectMissing(env, originalErr) {
		return nil, warnings, false, originalErr
	}
	sites, err := listSitesForRecovery(c, endpoint)
	if err != nil {
		return nil, warnings, false, originalErr
	}
	for _, site := range sites {
		if site.ID == id {
			return nil, warnings, false, originalErr
		}
	}
	data, err := json.Marshal(map[string]any{"id": id, "deleted": true})
	if err != nil {
		return nil, warnings, false, originalErr
	}
	warnings = append(warnings, fileStorageRecoveryWarning("delete", "site was deleted"))
	return &client.Envelope{Data: data}, warnings, true, nil
}

func isFileStorageObjectMissing(env *client.Envelope, err error) bool {
	if err == nil || env == nil || env.Err == nil {
		return false
	}
	return *env.Err == "object-not-found" && env.GetMsg() == fileStorageObjectMissing
}

func fileStorageRecoveryWarning(operation, result string) string {
	return fmt.Sprintf("SafeLine returned %q during site %s, but verification confirmed the %s", fileStorageObjectMissing, operation, result)
}

type recoverySite struct {
	ID   int
	Name string
	Raw  map[string]any
}

func listSitesForRecovery(c siteOperationClient, endpoint string) ([]recoverySite, error) {
	env, err := c.Do("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(env.Data, &raw); err != nil {
		var wrapped struct {
			Items []map[string]any `json:"items"`
		}
		if wrappedErr := json.Unmarshal(env.Data, &wrapped); wrappedErr != nil {
			return nil, err
		}
		raw = wrapped.Items
	}
	sites := make([]recoverySite, 0, len(raw))
	for _, item := range raw {
		sites = append(sites, recoverySite{ID: intNumber(item["id"]), Name: fmt.Sprint(item["name"]), Raw: item})
	}
	return sites, nil
}

func intNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}
