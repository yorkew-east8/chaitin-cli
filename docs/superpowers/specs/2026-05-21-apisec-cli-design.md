# APISec CLI Design

## Goal

Add an `apisec` product command to `chaitin-cli` using the version-aware OpenAPI and CLI mapping approach described in the product version maintenance design docs.

The first usable version should expose all exported APISec management APIs while giving better command names and help text for the highest-value APISec workflows.

## Product API Source

APISec source lives in `/Users/rui.zhu/Documents/workspace/04-开发/tools_llm/product_src/a-vatar`. The management backend is primarily under `skyview/skyview`.

The backend exports API classes derived from `utils.api.APIView`. Exported URLs are generated from class names, normally as `/api/<ClassName>`. Many request schemas are declared with `@serialize(<DRF Serializer>)`, which makes static OpenAPI generation feasible. Some endpoints read `request.GET` or `request.data` manually, so the generated schema must support a generic body fallback.

Authentication uses the `API-TOKEN` HTTP header. The CLI config should use this shape:

```yaml
apisec:
  url: https://your-apisec.example
  api_token: your-api-token
```

## Command Model

Use a two-layer command model.

The complete layer exposes every generated operation under `apisec raw`. This gives full API coverage even when no curated mapping exists.

The curated layer maps priority workflows into stable semantic commands through `cli-mapping.yaml`. These commands should have explicit names, accurate descriptions, and parameter help text that is safe for AI agents to follow.

Initial semantic command groups:

- `apisec asset api`: API assets, details, schema, data tags, status, remarks, import where feasible.
- `apisec asset site`: site listing, site details, move-to-app, enable or disable related operations.
- `apisec asset app`: application management, simple application lookup, priority, relocation.
- `apisec asset visitor`: visitor/source-IP query, groups, relationship charts, request and response summaries.
- `apisec asset config`: asset discovery config, success sign, identity, ignored API, resource recognition, effective scope.
- `apisec data rule`: data discovery rule configuration and built-in rule updates.
- `apisec data sensitive-transfer`: sensitive data transfer and conditional export workflows after the exact source endpoint is confirmed.
- `apisec risk config`: risk discovery config, risk model config, weak-password or weak-cipher config, whitelist.
- `apisec risk event`: risk event and event group query/update workflows.
- `apisec risk vulnerability`: vulnerability query and status update workflows.

## Version Layout

Follow the directory-as-plugin layout from the maintenance design:

```text
products/apisec/
  v26.05/
    openapi.json
    cli-mapping.yaml
  latest -> v26.05
```

The first iteration may embed the static `v26.05` schema and mapping. The loader should keep the boundary clear so a later APISec product-side schema endpoint can replace the embedded schema without changing command behavior.

## Help And AI Robustness

Help text is part of the product contract. Generated help must avoid ambiguous wording and should include:

- The exact APISec endpoint or operation ID for raw commands.
- Whether parameters are query parameters or JSON body fields.
- Required fields, default values, enum values, and array/object shape when known.
- A clear fallback rule: use `--body` or `--body-file` when a command has complex or unknown JSON input.
- Examples for priority semantic commands.

For raw commands, prefer mechanical and explicit help over friendly prose. For semantic commands, prefer business terms but retain operation ID references so an AI agent can map the command back to the API contract.

## Implementation Constraints

Keep product behavior in `products/apisec/`. Root command changes should only import and register the new product and apply runtime config. Shared OpenAPI/mapping code may be introduced only if it reduces duplication with existing generated products without broad rewrites.

No changes to the APISec source repository are required for the first iteration. Product-side schema and version endpoints are a future improvement.

## Known Risks

- Some APISec endpoints do not declare serializers, so the generated OpenAPI may be incomplete.
- Some GET endpoints manually inspect query keys, which may require mapping overrides or body/query fallback support.
- Export/download endpoints need special handling for file output.
- Semantic names may need product review to match APISec UI terminology.

## Acceptance Criteria

- `chaitin-cli apisec --help` clearly explains config, authentication, raw commands, semantic commands, and fallback input options.
- All generated APISec operations are reachable through `apisec raw`.
- The priority modules listed above have semantic command groups where the source schema is sufficient.
- Commands send `API-TOKEN` and honor `apisec.url` / `apisec.api_token` from config and equivalent flags.
- `task fmt`, `task test`, and relevant CLI help smoke tests pass.
