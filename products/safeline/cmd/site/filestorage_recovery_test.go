package site

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/chaitin/chaitin-cli/products/safeline/pkg/client"
)

type recoveryCall struct {
	method string
	path   string
}

type recoveryClient struct {
	env   *client.Envelope
	err   error
	calls []recoveryCall
}

func (c *recoveryClient) Do(method, path string, body io.Reader, query map[string]string) (*client.Envelope, error) {
	c.calls = append(c.calls, recoveryCall{method: method, path: path})
	return c.env, c.err
}

func fileStorageEnvelope() *client.Envelope {
	errCode := "object-not-found"
	return &client.Envelope{Err: &errCode, Msg: []byte(`"FileStorage: Object does not exist"`)}
}

func TestRecoverCreateFileStorageErrorTreatsExistingSiteAsSuccess(t *testing.T) {
	c := &recoveryClient{env: &client.Envelope{Data: []byte(`[{
		"id": 1752,
		"name": "site-a",
		"server_names": ["site-a.example.com"]
	}]`)}}
	originalErr := fmt.Errorf("API error [object-not-found]: FileStorage: Object does not exist")

	env, warnings, recovered, err := recoverCreateFileStorageError(c, "/api/SoftwareReverseProxyWebsiteAPI", map[string]any{"name": "site-a"}, fileStorageEnvelope(), originalErr, nil)
	if err != nil {
		t.Fatalf("recoverCreateFileStorageError: %v", err)
	}
	if !recovered {
		t.Fatalf("expected recovered")
	}
	if len(c.calls) != 1 || c.calls[0].method != "GET" || c.calls[0].path != "/api/SoftwareReverseProxyWebsiteAPI" {
		t.Fatalf("unexpected calls %+v", c.calls)
	}
	if !strings.Contains(string(env.Data), `"id":1752`) {
		t.Fatalf("expected recovered site in response, got %s", string(env.Data))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "FileStorage") {
		t.Fatalf("expected FileStorage warning, got %+v", warnings)
	}
}

func TestRecoverCreateFileStorageErrorKeepsOriginalErrorWhenSiteMissing(t *testing.T) {
	c := &recoveryClient{env: &client.Envelope{Data: []byte(`[]`)}}
	originalErr := fmt.Errorf("API error [object-not-found]: FileStorage: Object does not exist")

	_, _, recovered, err := recoverCreateFileStorageError(c, "/api/SoftwareReverseProxyWebsiteAPI", map[string]any{"name": "site-a"}, fileStorageEnvelope(), originalErr, nil)
	if err == nil || err != originalErr {
		t.Fatalf("expected original error, got %v", err)
	}
	if recovered {
		t.Fatalf("expected not recovered")
	}
}

func TestRecoverDeleteFileStorageErrorTreatsMissingSiteAsSuccess(t *testing.T) {
	c := &recoveryClient{env: &client.Envelope{Data: []byte(`[{"id": 12, "name": "other"}]`)}}
	originalErr := fmt.Errorf("API error [object-not-found]: FileStorage: Object does not exist")

	env, warnings, recovered, err := recoverDeleteFileStorageError(c, "/api/SoftwareReverseProxyWebsiteAPI", 42, fileStorageEnvelope(), originalErr, nil)
	if err != nil {
		t.Fatalf("recoverDeleteFileStorageError: %v", err)
	}
	if !recovered {
		t.Fatalf("expected recovered")
	}
	if !strings.Contains(string(env.Data), `"id":42`) {
		t.Fatalf("expected deleted id in response, got %s", string(env.Data))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "FileStorage") {
		t.Fatalf("expected FileStorage warning, got %+v", warnings)
	}
}

func TestRecoverDeleteFileStorageErrorKeepsOriginalErrorWhenSiteStillExists(t *testing.T) {
	c := &recoveryClient{env: &client.Envelope{Data: []byte(`[{"id": 42, "name": "site-a"}]`)}}
	originalErr := fmt.Errorf("API error [object-not-found]: FileStorage: Object does not exist")

	_, _, recovered, err := recoverDeleteFileStorageError(c, "/api/SoftwareReverseProxyWebsiteAPI", 42, fileStorageEnvelope(), originalErr, nil)
	if err == nil || err != originalErr {
		t.Fatalf("expected original error, got %v", err)
	}
	if recovered {
		t.Fatalf("expected not recovered")
	}
}

func TestFileStorageRecoveryDoesNotMatchOtherObjectNotFoundErrors(t *testing.T) {
	c := &recoveryClient{env: &client.Envelope{Data: []byte(`[{"id": 42, "name": "site-a"}]`)}}
	errCode := "object-not-found"
	env := &client.Envelope{Err: &errCode, Msg: []byte(`"Website: Object does not exist"`)}
	originalErr := fmt.Errorf("API error [object-not-found]: Website: Object does not exist")

	_, _, recovered, err := recoverCreateFileStorageError(c, "/api/SoftwareReverseProxyWebsiteAPI", map[string]any{"name": "site-a"}, env, originalErr, nil)
	if err == nil || err != originalErr {
		t.Fatalf("expected original error, got %v", err)
	}
	if recovered {
		t.Fatalf("expected not recovered")
	}
	if len(c.calls) != 0 {
		t.Fatalf("non-FileStorage errors must not trigger verification calls, got %+v", c.calls)
	}
}
