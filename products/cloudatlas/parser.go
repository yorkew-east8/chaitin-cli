package cloudatlas

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type Parser struct {
	api *OpenAPI
}

func ParseSpec(data []byte) (*OpenAPI, error) {
	var api OpenAPI
	if err := yaml.Unmarshal(data, &api); err != nil {
		return nil, fmt.Errorf("parse Cloud Atlas OpenAPI spec: %w", err)
	}
	return &api, nil
}

func NewParser(api *OpenAPI) *Parser {
	return &Parser{api: api}
}

func (p *Parser) GenerateCommands() ([]*cobra.Command, error) {
	parents := make(map[string]*cobra.Command)
	used := make(map[string]int)

	for _, path := range sortedPathKeys(p.api.Paths) {
		pathItem := p.api.Paths[path]
		for _, item := range operationsForPath(pathItem) {
			if item.op == nil {
				continue
			}
			segments := commandPath(item.op)
			if len(segments) == 0 {
				segments = pathCommandPath(path)
			}
			leaf := actionName(item.op.Summary)
			if leaf == "" {
				leaf = strings.ToLower(item.method)
			}

			parent := getOrCreateRoot(parents, segments[0])
			current := parent
			for _, segment := range segments[1:] {
				current = getOrCreateChild(current, segment)
			}

			key := strings.Join(append(segments, leaf), " ")
			used[key]++
			use := leaf
			if used[key] > 1 {
				use = leaf + "-" + uniqueSuffix(path, item.op)
			}
			current.AddCommand(p.createOperationCommand(use, item.method, path, item.op))
		}
	}

	commands := make([]*cobra.Command, 0, len(parents))
	for _, key := range sortedCommandKeys(parents) {
		commands = append(commands, parents[key])
	}
	return commands, nil
}

type pathOperation struct {
	method string
	op     *Operation
}

func operationsForPath(pathItem PathItem) []pathOperation {
	return []pathOperation{
		{method: "GET", op: pathItem.Get},
		{method: "POST", op: pathItem.Post},
		{method: "PUT", op: pathItem.Put},
		{method: "DELETE", op: pathItem.Delete},
		{method: "PATCH", op: pathItem.Patch},
	}
}

func sortedPathKeys(paths map[string]PathItem) []string {
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedCommandKeys(commands map[string]*cobra.Command) []string {
	keys := make([]string, 0, len(commands))
	for key := range commands {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func commandPath(op *Operation) []string {
	if len(op.Tags) == 0 || strings.TrimSpace(op.Tags[0]) == "" {
		return nil
	}
	parts := strings.Split(op.Tags[0], "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		segment := translateSegment(strings.TrimSpace(part))
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func pathCommandPath(path string) []string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.HasPrefix(part, "{") || part == "v1" || part == "api" {
			continue
		}
		segments = append(segments, normalizeSegment(part))
	}
	return segments
}

var segmentMap = map[string]string{
	"资产":        "asset",
	"种子":        "seed",
	"企业主体":      "enterprise",
	"关键词":       "keyword",
	"域名 WHOIS":  "domain-whois",
	"域名WHOIS":   "domain-whois",
	"邮箱域名":      "email-domain",
	"证书信息":      "certificate-info",
	"网站图标":      "favicon",
	"网站标题":      "title",
	"主域名":       "root-domain",
	"子域名情报":     "subdomain-intel",
	"子域名":       "subdomain",
	"IP地址":      "ip",
	"证书":        "certificate",
	"暴露面":       "exposure",
	"端口服务":      "port-service",
	"全端口":       "full-port",
	"网站实体":      "website",
	"网站路径":      "website-path",
	"网站指纹":      "website-fingerprint",
	"爬虫数据":      "crawler",
	"策略":        "strategy",
	"产品指纹":      "product-fingerprint",
	"漏洞列表":      "vulnerability",
	"风险":        "risk",
	"高危应用":      "high-risk-app",
	"自定义高危应用":   "custom-high-risk-app",
	"高危端口":      "high-risk-port",
	"自定义高危端口服务": "custom-high-risk-port",
	"管理后台":      "admin-panel",
	"数字情报":      "intelligence",
	"监控规则":      "monitor-rule",
	"任务管理":      "task-management",
	"周期任务管理":    "schedule-management",
	"开发社区泄露":    "code-leak",
	"排除策略":      "exclude-policy",
	"网盘泄露":      "cloud-drive-leak",
	"文库泄露":      "doc-leak",
	"暗网情报":      "dark-web",
	"失窃数据":      "stolen-data",
	"企业邮箱":      "enterprise-email",
	"移动应用":      "mobile-app",
	"新媒体":       "new-media",
	"任务":        "task",
	"周期任务v2":    "schedule",
	"任务实例v2":    "task-instance",
	"编排实例v2":    "orchestration-instance",
}

func translateSegment(value string) string {
	if translated, ok := segmentMap[value]; ok {
		return translated
	}
	return normalizeSegment(value)
}

func actionName(summary string) string {
	summary = strings.TrimSpace(summary)
	rules := []struct {
		contains string
		name     string
	}{
		{"批量修改监控开关", "set-monitor"},
		{"批量修改启用开关", "set-enabled"},
		{"批量修改置信度", "set-confidence"},
		{"批量修改可信标识", "set-trusted"},
		{"批量修改启用状态", "set-enabled"},
		{"批量切换状态", "set-status"},
		{"批量修改状态", "set-status"},
		{"批量绑定标签", "bind-tags"},
		{"批量解绑标签", "unbind-tags"},
		{"更新标签", "update-tags"},
		{"批量关联分组", "bind-group"},
		{"批量重新扫描", "rescan"},
		{"批量终止任务", "terminate"},
		{"批量重跑", "rerun"},
		{"批量删除", "delete"},
		{"批量添加", "batch-add"},
		{"批量创建", "batch-create"},
		{"立即运行", "run"},
		{"状态数量", "status-count"},
		{"状态选项", "status-options"},
		{"风险标签选项", "risk-tag-options"},
		{"分组选项", "group-options"},
		{"标签选项", "tag-options"},
		{"资产对象选项", "asset-options"},
		{"菜单", "options"},
		{"选项", "options"},
		{"列表", "list"},
		{"获取", "get"},
		{"查看", "get"},
		{"详情", "get"},
		{"查询", "get"},
		{"编辑", "update"},
		{"更新", "update"},
		{"添加", "create"},
		{"创建", "create"},
		{"开放", "open"},
	}
	for _, rule := range rules {
		if strings.Contains(summary, rule.contains) {
			return rule.name
		}
	}
	return normalizeSegment(summary)
}

func getOrCreateRoot(parents map[string]*cobra.Command, name string) *cobra.Command {
	if parents[name] != nil {
		return parents[name]
	}
	cmd := &cobra.Command{Use: name, Short: fmt.Sprintf("%s commands", name)}
	parents[name] = cmd
	return cmd
}

func getOrCreateChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	cmd := &cobra.Command{Use: name, Short: fmt.Sprintf("%s commands", name)}
	parent.AddCommand(cmd)
	return cmd
}

func (p *Parser) createOperationCommand(use, method, path string, op *Operation) *cobra.Command {
	short := strings.TrimSpace(op.Summary)
	if short == "" {
		short = fmt.Sprintf("%s %s", method, path)
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  buildOperationHelp(method, path, op),
		RunE: func(cmd *cobra.Command, args []string) error {
			return p.executeCommand(cmd, method, path, op)
		},
	}

	for _, param := range op.Parameters {
		if param.In != "query" && param.In != "path" {
			continue
		}
		flagName := normalizeFlagName(param.Name)
		required := param.Required || (param.In == "query" && flagName == "space")
		cmd.Flags().String(flagName, "", formatParameterUsage(param, required))
	}
	if !hasQueryParameter(op.Parameters, "space") {
		cmd.Flags().String("space", "", "空间ID；必填；可由 --space-id 或 cloudAtlas.space_id 提供默认值")
	}
	extraQueryFlag := "query"
	if cmd.Flags().Lookup(extraQueryFlag) != nil {
		extraQueryFlag = "query-param"
	}
	cmd.Flags().StringArray(extraQueryFlag, nil, "附加 query 参数，格式 key=value，可重复；用于传入 OpenAPI 未建模的查询参数")
	if op.RequestBody != nil {
		cmd.Flags().String("body", "", "JSON request body；字段说明见本命令 help 的 Body 区域")
		cmd.Flags().String("body-file", "", "读取 JSON request body 的文件路径；字段说明见本命令 help 的 Body 区域")
	}
	if requiresConfirmation(method, op.Summary) {
		cmd.Flags().Bool("yes", false, "Confirm this modifying operation")
	}
	return cmd
}

func (p *Parser) executeCommand(cmd *cobra.Command, method, path string, op *Operation) error {
	if requiresConfirmation(method, op.Summary) {
		yes, err := cmd.Flags().GetBool("yes")
		if err != nil {
			return err
		}
		if !yes {
			return fmt.Errorf("operation %q requires --yes", op.Summary)
		}
	}

	requestPath, err := buildPath(cmd, path, op.Parameters)
	if err != nil {
		return err
	}
	query, err := buildQuery(cmd, op.Parameters)
	if err != nil {
		return err
	}
	body, err := buildBody(cmd, op)
	if err != nil {
		return err
	}

	cfg := getConfigFromCommand(cmd)
	client := NewClient(cfg, runtimeInsecure, verbose)
	data, err := client.Do(cmd.Context(), method, requestPath, query, body)
	if err != nil {
		if _, ok := err.(dryRunResult); !ok {
			return err
		}
	}
	return NewRenderer(outputFormat, cmd.OutOrStdout()).Render(data)
}

func buildPath(cmd *cobra.Command, path string, params []Parameter) (string, error) {
	result := path
	for _, param := range params {
		if param.In != "path" {
			continue
		}
		flagName := normalizeFlagName(param.Name)
		value, err := cmd.Flags().GetString(flagName)
		if err != nil {
			return "", err
		}
		if value == "" && param.Required {
			return "", fmt.Errorf("--%s is required", flagName)
		}
		result = strings.ReplaceAll(result, "{"+param.Name+"}", url.PathEscape(value))
	}
	return result, nil
}

func buildQuery(cmd *cobra.Command, params []Parameter) (url.Values, error) {
	query := url.Values{}
	spaceHandled := false
	for _, param := range params {
		if param.In != "query" {
			continue
		}
		flagName := normalizeFlagName(param.Name)
		if flagName == "space" {
			spaceHandled = true
		}
		value, err := cmd.Flags().GetString(flagName)
		if err != nil {
			return nil, err
		}
		if flagName == "space" && value == "" {
			value = spaceIDFromCommand(cmd)
		}
		required := param.Required || flagName == "space"
		if value == "" {
			if required {
				return nil, fmt.Errorf("--%s is required", flagName)
			}
			continue
		}
		query.Set(param.Name, value)
	}
	if !spaceHandled {
		space, err := cmd.Flags().GetString("space")
		if err != nil {
			return nil, err
		}
		if space == "" {
			space = spaceIDFromCommand(cmd)
		}
		if space == "" {
			return nil, fmt.Errorf("--space or --space-id is required")
		}
		query.Set("space", space)
	}
	additionalFlag := "query"
	if cmd.Flags().Lookup(additionalFlag) == nil || isOperationParameter(params, "query") {
		if cmd.Flags().Lookup("query-param") != nil {
			additionalFlag = "query-param"
		}
	}
	pairs, err := cmd.Flags().GetStringArray(additionalFlag)
	if err != nil {
		return nil, err
	}
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid --query %q, expected key=value", pair)
		}
		query.Add(key, value)
	}
	return query, nil
}

func hasQueryParameter(params []Parameter, name string) bool {
	for _, param := range params {
		if param.In == "query" && normalizeFlagName(param.Name) == name {
			return true
		}
	}
	return false
}

func isOperationParameter(params []Parameter, flagName string) bool {
	for _, param := range params {
		if normalizeFlagName(param.Name) == flagName {
			return true
		}
	}
	return false
}

func buildBody(cmd *cobra.Command, op *Operation) (any, error) {
	if op.RequestBody == nil {
		return nil, nil
	}
	bodyText, err := cmd.Flags().GetString("body")
	if err != nil {
		return nil, err
	}
	bodyFile, err := cmd.Flags().GetString("body-file")
	if err != nil {
		return nil, err
	}
	if bodyText != "" && bodyFile != "" {
		return nil, fmt.Errorf("--body and --body-file cannot be used together")
	}
	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("read body file: %w", err)
		}
		bodyText = string(data)
	}
	if bodyText == "" {
		return nil, nil
	}
	var body any
	if err := json.Unmarshal([]byte(bodyText), &body); err != nil {
		return nil, fmt.Errorf("parse body JSON: %w", err)
	}
	return body, nil
}

func getConfigFromCommand(cmd *cobra.Command) Config {
	cfg := runtimeCfg
	if value, err := cmd.Flags().GetString("url"); err == nil && value != "" {
		cfg.URL = value
	}
	if value, err := cmd.Flags().GetString("token"); err == nil && value != "" {
		cfg.Token = value
	}
	if value := spaceIDFromCommand(cmd); value != "" {
		cfg.SpaceID = value
	}
	return cfg
}

func spaceIDFromCommand(cmd *cobra.Command) string {
	if value, err := cmd.Flags().GetString("space-id"); err == nil && value != "" {
		return value
	}
	if value, err := cmd.InheritedFlags().GetString("space-id"); err == nil && value != "" {
		return value
	}
	return runtimeCfg.SpaceID
}

func requiresConfirmation(method, summary string) bool {
	if method == "DELETE" {
		return true
	}
	for _, keyword := range []string{"删除", "修改状态", "修改监控开关", "修改启用开关", "修改启用状态", "修改置信度", "切换状态", "终止", "重跑", "解绑"} {
		if strings.Contains(summary, keyword) {
			return true
		}
	}
	return false
}

func buildOperationHelp(method, path string, op *Operation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\nEndpoint: %s %s\n", op.Summary, method, path)
	if op.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", op.Description)
	}
	if len(op.Parameters) > 0 {
		fmt.Fprintln(&b, "\nParameters:")
		for _, param := range op.Parameters {
			required := param.Required || (param.In == "query" && normalizeFlagName(param.Name) == "space")
			fmt.Fprintf(&b, "  --%s (%s): %s\n", normalizeFlagName(param.Name), param.In, formatParameterUsage(param, required))
		}
	}
	if op.RequestBody != nil {
		fmt.Fprintln(&b, "\nBody: use --body or --body-file with JSON input.")
		bodyHelp := formatRequestBodyHelp(op.RequestBody)
		if bodyHelp != "" {
			fmt.Fprintf(&b, "%s\n", bodyHelp)
		}
	}
	return strings.TrimSpace(b.String())
}

func formatParameterUsage(param Parameter, required bool) string {
	parts := make([]string, 0, 6)
	description := strings.TrimSpace(param.Description)
	if description == "" {
		description = "OpenAPI 未提供说明"
	}
	parts = append(parts, description)
	if required {
		parts = append(parts, "必填")
	}
	parts = append(parts, formatSchemaDetails(param.Schema, param.Example)...)
	if normalizeFlagName(param.Name) == "space" {
		parts = append(parts, "可由 --space-id 或 cloudAtlas.space_id 提供默认值")
	}
	return strings.Join(parts, "；")
}

func formatSchemaDetails(schema *Schema, example any) []string {
	if schema == nil {
		return nil
	}
	parts := make([]string, 0, 4)
	if typ := schemaTypeName(schema); typ != "" {
		parts = append(parts, "类型: "+typ)
	}
	if defaultText := valueHelp(schema.Default); defaultText != "" {
		parts = append(parts, "默认值: "+defaultText)
	}
	if exampleText := valueHelp(firstNonNil(example, schema.Example)); exampleText != "" {
		parts = append(parts, "示例: "+exampleText)
	}
	if enumText := formatEnumValues(schema); enumText != "" {
		parts = append(parts, "可选值: "+enumText)
	}
	return parts
}

func schemaTypeName(schema *Schema) string {
	if schema == nil {
		return ""
	}
	if schema.Type != "" {
		if schema.Nullable {
			return schema.Type + "|null"
		}
		return schema.Type
	}
	if schema.Ref != "" {
		return schema.Ref
	}
	if len(schema.Properties) > 0 {
		return "object"
	}
	if schema.Items != nil {
		return "array"
	}
	return ""
}

func formatEnumValues(schema *Schema) string {
	if schema == nil || len(schema.Enum) == 0 {
		return ""
	}
	descriptions := enumDescriptions(schema)
	items := make([]string, 0, len(schema.Enum))
	for _, value := range schema.Enum {
		valueText := valueHelp(value)
		if valueText == "" {
			continue
		}
		if description := strings.TrimSpace(descriptions[valueText]); description != "" {
			items = append(items, valueText+"="+description)
			continue
		}
		items = append(items, valueText)
	}
	return strings.Join(items, ", ")
}

func enumDescriptions(schema *Schema) map[string]string {
	descriptions := make(map[string]string)
	if schema == nil {
		return descriptions
	}
	for value, description := range schema.XApifox.EnumDescriptions {
		if strings.TrimSpace(description) != "" {
			descriptions[value] = strings.TrimSpace(description)
		}
	}
	for _, item := range schema.XApifoxEnum {
		value := valueHelp(item.Value)
		if value == "" {
			continue
		}
		description := strings.TrimSpace(item.Description)
		if description == "" {
			description = strings.TrimSpace(item.Name)
		}
		if description != "" {
			descriptions[value] = description
		}
	}
	return descriptions
}

func formatRequestBodyHelp(body *RequestBody) string {
	if body == nil {
		return ""
	}
	media := body.Content["application/json"]
	if media.Schema == nil {
		for _, candidate := range body.Content {
			media = candidate
			break
		}
	}
	lines := formatSchemaFields(media.Schema, "  ")
	if len(lines) == 0 {
		return "  OpenAPI 未提供可展开的 body 字段说明。"
	}
	return strings.Join(lines, "\n")
}

func formatSchemaFields(schema *Schema, indent string) []string {
	if schema == nil || len(schema.Properties) == 0 {
		return nil
	}
	required := make(map[string]struct{}, len(schema.Required))
	for _, name := range schema.Required {
		required[name] = struct{}{}
	}
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		prop := schema.Properties[name]
		parts := make([]string, 0, 6)
		if description := strings.TrimSpace(prop.Description); description != "" {
			parts = append(parts, description)
		} else {
			parts = append(parts, "OpenAPI 未提供说明")
		}
		if _, ok := required[name]; ok {
			parts = append(parts, "必填")
		}
		parts = append(parts, formatSchemaDetails(&prop, prop.Example)...)
		lines = append(lines, fmt.Sprintf("%s%s: %s", indent, name, strings.Join(parts, "；")))
	}
	return lines
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func valueHelp(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return strings.TrimSpace(string(data))
	}
}

func normalizeFlagName(name string) string {
	return strings.ReplaceAll(normalizeSegment(name), "_", "-")
}

func normalizeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ToLower(value)

	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' && !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

func uniqueSuffix(path string, op *Operation) string {
	if op.OperationID != "" {
		return normalizeSegment(op.OperationID)
	}
	cleaned := pathParamPattern.ReplaceAllString(path, "$1")
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if segment := normalizeSegment(parts[i]); segment != "" && segment != "v1" {
			return segment
		}
	}
	return "operation"
}
