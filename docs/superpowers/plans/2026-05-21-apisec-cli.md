# APISec CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an `apisec` product command that exposes all generated APISec APIs through `raw` commands and priority workflows through semantic commands with AI-safe help text.

**Architecture:** Build a product-local OpenAPI/mapping loader under `products/apisec/`. Generate a static APISec OpenAPI document from the APISec Django `APIView` source, embed it with `cli-mapping.yaml`, and generate Cobra commands at startup. Keep raw command generation exhaustive and semantic command generation mapping-driven.

**Tech Stack:** Go, Cobra, embedded files, YAML/JSON, standard `net/http`, existing `config` package, Python source introspection for one-time OpenAPI generation.

---

## File Structure

- Create `products/apisec/types.go`: APISec config, OpenAPI, mapping, and command metadata types.
- Create `products/apisec/client.go`: HTTP client with `API-TOKEN` authentication, JSON request handling, dry-run support, and response parsing.
- Create `products/apisec/render.go`: table/json output rendering, initially JSON-first for predictable AI consumption.
- Create `products/apisec/schema_loader.go`: embedded `v26.05` schema and mapping loading.
- Create `products/apisec/parser.go`: raw command generation and semantic mapping command generation.
- Create `products/apisec/command.go`: `apisec` root command, flags, runtime config application, command registration.
- Create `products/apisec/testdata/openapi_minimal.json`: compact OpenAPI fixture for parser tests.
- Create `products/apisec/testdata/cli-mapping.yaml`: compact mapping fixture for semantic command tests.
- Create `products/apisec/v26.05/openapi.json`: generated static APISec schema.
- Create `products/apisec/v26.05/cli-mapping.yaml`: first semantic mapping for priority modules.
- Create `products/apisec/latest`: text file containing `v26.05`; use this instead of a symlink so Go embed works consistently.
- Create `tools/apisec-openapi-gen/README.md`: describe how the schema was generated and what it cannot infer.
- Create `tools/apisec-openapi-gen/generate.py`: source scanner that emits the first OpenAPI document from APISec `APIView` classes.
- Modify `main.go`: import and register `products/apisec`; apply runtime config in `wrapProductCommand`.
- Modify `README.md` or `config.yaml.example` only if an existing config example section exists and the change is small.

## Task 1: APISec Types And Fixtures

**Files:**
- Create: `products/apisec/types.go`
- Create: `products/apisec/testdata/openapi_minimal.json`
- Create: `products/apisec/testdata/cli-mapping.yaml`
- Test: `products/apisec/schema_loader_test.go`

- [ ] **Step 1: Add failing loader tests**

Create `products/apisec/schema_loader_test.go` with tests that load explicit fixture bytes, assert OpenAPI paths are parsed, and assert mapping command paths resolve to operation IDs.

```go
package apisec

import "testing"

func TestParseOpenAPI(t *testing.T) {
	api, err := parseOpenAPI([]byte(`{"openapi":"3.0.3","info":{"title":"APISec","version":"26.05"},"paths":{"/api/ApplicationAPI":{"get":{"operationId":"ApplicationAPI_get","summary":"List applications"}}}}`))
	if err != nil {
		t.Fatalf("parseOpenAPI() error = %v", err)
	}
	if api.Paths["/api/ApplicationAPI"].Get.OperationID != "ApplicationAPI_get" {
		t.Fatalf("operationId not parsed: %+v", api.Paths["/api/ApplicationAPI"].Get)
	}
}

func TestParseCLIMapping(t *testing.T) {
	mapping, err := parseCLIMapping([]byte(`commands:
  - path: [asset, app, list]
    operationId: ApplicationAPI_get
    short: List applications
`))
	if err != nil {
		t.Fatalf("parseCLIMapping() error = %v", err)
	}
	if got := mapping.Commands[0].Path[2]; got != "list" {
		t.Fatalf("path[2] = %q, want list", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./products/apisec`

Expected: fail because `products/apisec` and parser functions do not exist yet.

- [ ] **Step 3: Implement types and parse helpers**

Create `products/apisec/types.go` with `Config`, `OpenAPI`, `PathItem`, `Operation`, `Parameter`, `Schema`, `RequestBody`, `CLIMapping`, and `MappedCommand`. Include `operationId`, request body, schema descriptions, enum, items, and properties fields because help generation needs them.

Create `products/apisec/schema_loader.go` with:

```go
func parseOpenAPI(data []byte) (*OpenAPI, error)
func parseCLIMapping(data []byte) (*CLIMapping, error)
```

Use `encoding/json` for OpenAPI and `gopkg.in/yaml.v3` for mapping.

- [ ] **Step 4: Add fixtures**

Add `products/apisec/testdata/openapi_minimal.json` with one GET and one POST operation. Add `products/apisec/testdata/cli-mapping.yaml` with one mapped semantic command and one raw-hidden false default.

- [ ] **Step 5: Run tests**

Run: `go test ./products/apisec`

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add products/apisec/types.go products/apisec/schema_loader.go products/apisec/schema_loader_test.go products/apisec/testdata
git commit -m "feat: add apisec schema types"
```

## Task 2: APISec Client And Renderer

**Files:**
- Create: `products/apisec/client.go`
- Create: `products/apisec/client_test.go`
- Create: `products/apisec/render.go`
- Create: `products/apisec/render_test.go`

- [ ] **Step 1: Add client tests**

Create tests with `httptest.Server` that verify `API-TOKEN` is sent, JSON body is encoded, query parameters are preserved, and APISec response envelopes render `data` when `err` is null.

Test cases:

- `TestClientSetsAPITokenHeader`: server asserts `r.Header.Get("API-TOKEN") == "token-1"`.
- `TestClientSendsJSONBody`: server decodes body `{"name":"app"}`.
- `TestClientReturnsAPIError`: server returns `{"err":"bad-request","msg":"invalid"}` and client returns an error containing `bad-request`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./products/apisec -run 'TestClient|TestRenderer'`

Expected: fail because client and renderer do not exist.

- [ ] **Step 3: Implement client**

Implement `NewClient(cfg *Config, verbose bool) *Client` and `Do(ctx, method, path string, query url.Values, body any, result any) error`.

Rules:

- Trim trailing slash from `Config.URL`.
- Set `API-TOKEN` from `Config.APIToken` when non-empty.
- Set `Content-Type: application/json` only when body is non-nil.
- Treat HTTP status `>=400` as an error.
- For APISec response envelopes, if JSON has non-empty `err`, return an API error containing `err` and `msg`; otherwise expose `data` to the renderer.
- Respect package-level `dryRun` by printing request details and not sending the request.

- [ ] **Step 4: Implement renderer**

Support `--output json` and `--output table`. For first iteration, table may pretty-print compact JSON with stable indentation when data is not a simple array/object shape. JSON output must emit valid JSON to stdout.

- [ ] **Step 5: Run tests**

Run: `go test ./products/apisec -run 'TestClient|TestRenderer'`

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add products/apisec/client.go products/apisec/client_test.go products/apisec/render.go products/apisec/render_test.go
git commit -m "feat: add apisec client"
```

## Task 3: Raw Command Parser

**Files:**
- Create: `products/apisec/parser.go`
- Create: `products/apisec/parser_test.go`

- [ ] **Step 1: Add parser tests**

Create tests using the minimal fixture to assert:

- `GenerateRawCommands` creates `raw application-api get` for `/api/ApplicationAPI` GET.
- Help for raw command includes `Endpoint: GET /api/ApplicationAPI` and `Operation ID: ApplicationAPI_get`.
- Query parameters become flags with descriptions and required markers.
- Request bodies add `--body` and `--body-file`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./products/apisec -run TestGenerateRawCommands`

Expected: fail because parser does not exist.

- [ ] **Step 3: Implement raw command generation**

Implement:

```go
func NewParser(api *OpenAPI, mapping *CLIMapping) *Parser
func (p *Parser) GenerateRawCommands() ([]*cobra.Command, error)
```

Raw command naming:

- Convert API class name to kebab case: `ApplicationAPI` -> `application-api`.
- Child command is HTTP verb: `get`, `post`, `put`, `delete`, `patch`.
- If multiple operations would collide, append operation ID suffix.

Execution rules:

- Path parameters replace `{name}`.
- Query parameters go into URL query.
- Header parameters are allowed except managed `API-TOKEN`, `Content-Type`, and `Accept`.
- Body comes from `--body` or `--body-file`; `--body-file` wins.

Help rules:

- Long help includes endpoint, operation ID, summary, parameter locations, required fields, and body fallback guidance.
- Do not infer business meaning for raw commands.

- [ ] **Step 4: Run parser tests**

Run: `go test ./products/apisec -run TestGenerateRawCommands`

Expected: pass.

- [ ] **Step 5: Commit**

Run:

```bash
git add products/apisec/parser.go products/apisec/parser_test.go
git commit -m "feat: generate apisec raw commands"
```

## Task 4: Semantic Mapping Commands

**Files:**
- Modify: `products/apisec/parser.go`
- Modify: `products/apisec/parser_test.go`
- Modify: `products/apisec/types.go`

- [ ] **Step 1: Add semantic command tests**

Add tests that load mapping fixture and assert:

- `asset app list` is generated.
- It executes the same operation as `ApplicationAPI_get`.
- Its help includes the business short text and `Operation ID: ApplicationAPI_get`.
- A mapped command can rename flags through mapping entries.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./products/apisec -run TestGenerateSemanticCommands`

Expected: fail because semantic generation does not exist.

- [ ] **Step 3: Extend mapping types**

Support this mapping shape:

```yaml
commands:
  - path: [asset, app, list]
    operationId: ApplicationAPI_get
    short: List applications
    long: List APISec applications.
    examples:
      - chaitin-cli apisec asset app list --output json
    flags:
      page:
        name: page
        description: Page number.
```

- [ ] **Step 4: Implement semantic command generation**

Implement `GenerateSemanticCommands`. Build parent commands from mapping paths. Reuse operation execution from raw commands. Apply mapped short, long, examples, and flag descriptions. Keep operation ID references in long help.

- [ ] **Step 5: Run tests**

Run: `go test ./products/apisec -run 'TestGenerateSemanticCommands|TestGenerateRawCommands'`

Expected: pass.

- [ ] **Step 6: Commit**

Run:

```bash
git add products/apisec/parser.go products/apisec/parser_test.go products/apisec/types.go
git commit -m "feat: map apisec semantic commands"
```

## Task 5: Product Command Integration

**Files:**
- Create: `products/apisec/command.go`
- Create: `products/apisec/command_test.go`
- Modify: `main.go`

- [ ] **Step 1: Add command tests**

Test that `NewCommand()` has persistent flags `--url`, `--api-token`, `--output`, `--verbose`, and that runtime config applies values when flags were not set.

Test that `apisec --help` mentions `API-TOKEN`, `raw`, `--body`, and `--body-file`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./products/apisec -run TestNewCommand`

Expected: fail because command root does not exist.

- [ ] **Step 3: Implement product command**

Create `NewCommand() *cobra.Command`, `ApplyRuntimeConfig(cmd *cobra.Command, cfg config.Raw, isDryRun bool)`, and internal `applyRuntimeConfig`.

Root command behavior:

- `Use: "apisec"`
- Long help explains config, `API-TOKEN`, raw commands, semantic commands, and body fallback.
- Persistent flags: `--url`, `--api-token`, `--output table|json`, `--verbose`.
- Load embedded schema and mapping.
- Add `raw` parent command containing full raw operations.
- Add semantic commands at root.

- [ ] **Step 4: Register product in root**

Modify `main.go`:

- Import `github.com/chaitin/chaitin-cli/products/apisec`.
- Call `a.registerProductCommand(apisec.NewCommand())` in `newApp()`.
- Add `case "apisec": apisec.ApplyRuntimeConfig(command, a.config, a.dryRun)` in `wrapProductCommand`.

- [ ] **Step 5: Run tests**

Run: `go test ./products/apisec ./...`

Expected: pass or reveal unrelated existing failures that must be reported before continuing.

- [ ] **Step 6: Commit**

Run:

```bash
git add products/apisec/command.go products/apisec/command_test.go main.go
git commit -m "feat: register apisec product command"
```

## Task 6: APISec OpenAPI Generator

**Files:**
- Create: `tools/apisec-openapi-gen/README.md`
- Create: `tools/apisec-openapi-gen/generate.py`
- Create: `products/apisec/v26.05/openapi.json`
- Create: `products/apisec/latest`
- Modify: `products/apisec/schema_loader.go`

- [ ] **Step 1: Add generator README**

Document source path, command, known limits, and output path:

```bash
python3 tools/apisec-openapi-gen/generate.py \
  --source /Users/rui.zhu/Documents/workspace/04-开发/tools_llm/product_src/a-vatar/skyview/skyview \
  --output products/apisec/v26.05/openapi.json
```

- [ ] **Step 2: Implement source scanner**

Implement a conservative Python AST scanner:

- Find `class *API(...)` in `*/views.py`.
- Include methods named `get`, `post`, `put`, `delete`, `patch`.
- Detect nearest decorators `@serialize(SomeSerializer)` and `@serialize(SomeSerializer, serializer_many=True)`.
- Emit path `/api/<ClassName>`.
- Emit operation ID `<ClassName>_<method>`.
- Emit summary from class/method docstring when present, otherwise `<METHOD> /api/<ClassName>`.
- For serializer-backed methods, add JSON request body for non-GET and query parameters for GET when fields can be parsed from the serializer source.
- If fields cannot be parsed, still emit operation with `x-cli-body-fallback: true`.

- [ ] **Step 3: Generate schema**

Run the generator command from Step 1. Inspect that output contains priority classes such as `ApplicationAPI_get`, `InterfaceAPI_get`, `DetectorConfigAPI_get`, `DiscoverDataTaskAPI_post`, `RiskEventAPI_get`, and `VulnerabilityAPI_get`.

- [ ] **Step 4: Embed version layout**

Create `products/apisec/latest` containing exactly:

```text
v26.05
```

Update `schema_loader.go` to use `//go:embed latest v26.05/openapi.json v26.05/cli-mapping.yaml` and load the version named by `latest`.

- [ ] **Step 5: Run schema loader tests**

Run: `go test ./products/apisec -run TestLoadEmbeddedSchema`

Expected: pass after adding a test that embedded schema has non-empty paths.

- [ ] **Step 6: Commit**

Run:

```bash
git add tools/apisec-openapi-gen products/apisec/v26.05/openapi.json products/apisec/latest products/apisec/schema_loader.go products/apisec/schema_loader_test.go
git commit -m "feat: generate apisec openapi schema"
```

## Task 7: Priority CLI Mapping

**Files:**
- Create: `products/apisec/v26.05/cli-mapping.yaml`
- Modify: `products/apisec/parser_test.go`

- [ ] **Step 1: Add mapping smoke test**

Test embedded mapping contains commands for:

- `asset api`
- `asset site`
- `asset app`
- `asset visitor`
- `asset config`
- `data rule`
- `risk config`
- `risk event`
- `risk vulnerability`

- [ ] **Step 2: Create initial mapping**

Map priority operations discovered in APISec source:

- `ApplicationAPI_get/post/put/delete` under `asset app`.
- `SiteAPI_get/delete` and `SiteInfoAPI_put` under `asset site`.
- `InterfaceAPI_get/delete`, `InterfaceDetailsAPI_get`, `SchemaAPI_get`, `DataTagsAPI_get`, `InterfaceStatusAPI_get/put` under `asset api`.
- `UserSrcIpInfoQueryAPI_get`, `UserSrcIpListAPI_get`, `UserSrcIpGroupAPI_get`, `UserSrcIpAsRequesterAPI_get`, `UserSrcIpAsResponserAPI_get` under `asset visitor`.
- `DetectorConfigAPI_get/put`, `SuccessSignAPI_get/post/put/delete`, `IdentityAPI_get/post/put/delete`, `IgnoredInterfaceAPI_post/put`, `ResourceRecognitionBindingAPI_get/put` under `asset config`.
- `DiscoverDataTaskAPI_post/put`, `BuildInDiscoverDataTaskAPI_put`, `RefreshDesensitizeCacheAPI_post` under `data rule`.
- `SensitiveDataCharacteristicAPI_get` under `data sensitive-transfer` until exact export endpoint is confirmed.
- `RiskStrategyAPI_post/put`, `RiskModelConfigAPI_get/put`, `RiskFunctionStatusAPI_get`, `WeakerCipherAPI_post/delete`, `EventWhiteListAPI_post/put/delete`, `RiskStrategyAuxiliaryInfoAPI_get` under `risk config`.
- `RiskEventAPI_get`, `RiskEventGroupAPI_get/put` under `risk event`.
- `VulnerabilityAPI_get/put` under `risk vulnerability`.

- [ ] **Step 3: Run mapping tests**

Run: `go test ./products/apisec -run 'TestEmbeddedMapping|TestGenerateSemanticCommands'`

Expected: pass.

- [ ] **Step 4: Commit**

Run:

```bash
git add products/apisec/v26.05/cli-mapping.yaml products/apisec/parser_test.go
git commit -m "feat: map priority apisec workflows"
```

## Task 8: End-To-End Verification

**Files:**
- Modify as needed only in files touched by earlier tasks.

- [ ] **Step 1: Format**

Run: `task fmt`

Expected: all Go files formatted.

- [ ] **Step 2: Run tests**

Run: `task test`

Expected: all Go tests pass.

- [ ] **Step 3: Run lint**

Run: `task lint`

Expected: `go vet ./...` passes.

- [ ] **Step 4: Build**

Run: `task build`

Expected: `bin/chaitin-cli` builds.

- [ ] **Step 5: Help smoke tests**

Run:

```bash
bin/chaitin-cli apisec --help
bin/chaitin-cli apisec raw --help
bin/chaitin-cli apisec asset app --help
bin/chaitin-cli apisec risk event --help
```

Expected: help mentions raw coverage, semantic command groups, `API-TOKEN`, operation IDs for operation commands, and `--body` / `--body-file` where applicable.

- [ ] **Step 6: Dry-run smoke test**

Run a safe dry-run command against a placeholder URL, for example:

```bash
bin/chaitin-cli --dry-run apisec --url https://apisec.example --api-token token raw application-api get --output json
```

Expected: request method, URL, and `API-TOKEN` header are printed; no network request is sent.

- [ ] **Step 7: Final status**

Run: `git status --short --branch`

Expected: clean branch after final commit, or only intentional uncommitted verification artifacts.

- [ ] **Step 8: Commit final fixes if needed**

If verification required fixes, commit them:

```bash
git add products/apisec main.go tools/apisec-openapi-gen docs/superpowers
git commit -m "test: verify apisec cli"
```

## Self-Review Notes

- Spec coverage: raw full coverage is implemented in Tasks 3 and 6; semantic priority coverage is implemented in Tasks 4 and 7; help robustness is implemented and verified in Tasks 3, 4, 5, and 8; config/auth is implemented in Tasks 2 and 5.
- Scope risk: runtime product-side schema fetching is intentionally not part of this first implementation; the loader boundary in Task 6 keeps that path open.
- Known gap: sensitive data conditional export endpoint is not yet confirmed in source inspection, so the first mapping includes the closest known sensitive data endpoint and leaves exact export mapping to source confirmation during Task 7.
