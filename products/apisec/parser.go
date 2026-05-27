package apisec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

type Parser struct {
	api          *OpenAPI
	mapping      *CLIMapping
	operationMap map[string]*MappedCommand
}

type operationRef struct {
	method string
	path   string
	op     *Operation
}

func NewParser(api *OpenAPI, mapping *CLIMapping) *Parser {
	return &Parser{api: api, mapping: mapping, operationMap: mappedCommandsByOperationID(mapping)}
}

func mappedCommandsByOperationID(mapping *CLIMapping) map[string]*MappedCommand {
	result := make(map[string]*MappedCommand)
	if mapping == nil {
		return result
	}
	for i := range mapping.Commands {
		mapped := rawHelpMetadata(&mapping.Commands[i])
		if mapped.OperationID == "" {
			continue
		}
		if _, exists := result[mapped.OperationID]; !exists || hasHelpMetadata(mapped) {
			result[mapped.OperationID] = mapped
		}
	}
	return result
}

func rawHelpMetadata(mapped *MappedCommand) *MappedCommand {
	if mapped == nil {
		return nil
	}
	return &MappedCommand{
		OperationID: mapped.OperationID,
		Short:       mapped.Short,
		Long:        mapped.Long,
		Examples:    mapped.Examples,
		EnumHints:   mapped.EnumHints,
		BodyExample: mapped.BodyExample,
		Aliases:     mapped.Aliases,
		Rollback:    mapped.Rollback,
		Flags:       mapped.Flags,
		Metadata:    mapped.Metadata,
	}
}

func hasHelpMetadata(mapped *MappedCommand) bool {
	return mapped != nil && (len(mapped.EnumHints) > 0 || len(mapped.BodyExample) > 0 || len(mapped.Aliases) > 0 || mapped.Rollback != nil)
}

func (p *Parser) GenerateRawCommands() ([]*cobra.Command, error) {
	if p.api == nil {
		return nil, nil
	}
	parents := make(map[string]*cobra.Command)
	paths := sortedPathKeys(p.api.Paths)
	used := make(map[string]int)

	for _, path := range paths {
		pathItem := p.api.Paths[path]
		for _, item := range operationsForPath(pathItem) {
			if item.op == nil {
				continue
			}
			parentName := rawParentName(path)
			parent := parents[parentName]
			if parent == nil {
				parent = &cobra.Command{Use: parentName, Short: fmt.Sprintf("Raw commands for %s", parentName)}
				parents[parentName] = parent
			}

			childName := strings.ToLower(item.method)
			key := parentName + " " + childName
			used[key]++
			if used[key] > 1 {
				childName = childName + "-" + normalizeSegment(item.op.OperationID)
			}
			parent.AddCommand(p.createOperationCommand(childName, item.method, path, item.op, p.operationMap[item.op.OperationID]))
		}
	}

	commands := make([]*cobra.Command, 0, len(parents))
	for _, name := range sortedCommandKeys(parents) {
		commands = append(commands, parents[name])
	}
	return commands, nil
}

func (p *Parser) GenerateSemanticCommands() ([]*cobra.Command, error) {
	if p.api == nil || p.mapping == nil {
		return nil, nil
	}
	operations := p.operationsByID()
	parents := make(map[string]*cobra.Command)

	for _, mapped := range p.mapping.Commands {
		if len(mapped.Path) == 0 {
			continue
		}
		ref, ok := operations[mapped.OperationID]
		if !ok {
			continue
		}
		parent := getOrCreateMappedParent(parents, mapped.Path[0])
		current := parent
		for _, segment := range mapped.Path[1 : len(mapped.Path)-1] {
			current = getOrCreateChild(current, segment)
		}
		leafName := mapped.Path[len(mapped.Path)-1]
		current.AddCommand(p.createOperationCommand(leafName, ref.method, ref.path, ref.op, &mapped))
	}

	commands := make([]*cobra.Command, 0, len(parents))
	for _, name := range sortedCommandKeys(parents) {
		commands = append(commands, parents[name])
	}
	return commands, nil
}

func (p *Parser) operationsByID() map[string]operationRef {
	operations := make(map[string]operationRef)
	for _, path := range sortedPathKeys(p.api.Paths) {
		pathItem := p.api.Paths[path]
		for _, item := range operationsForPath(pathItem) {
			if item.op == nil || item.op.OperationID == "" {
				continue
			}
			operations[item.op.OperationID] = operationRef{method: item.method, path: path, op: item.op}
		}
	}
	return operations
}

func getOrCreateMappedParent(parents map[string]*cobra.Command, name string) *cobra.Command {
	name = normalizeSegment(name)
	if parents[name] != nil {
		return parents[name]
	}
	cmd := &cobra.Command{Use: name, Short: fmt.Sprintf("%s commands", name)}
	parents[name] = cmd
	return cmd
}

func getOrCreateChild(parent *cobra.Command, name string) *cobra.Command {
	name = normalizeSegment(name)
	for _, cmd := range parent.Commands() {
		if cmd.Use == name {
			return cmd
		}
	}
	cmd := &cobra.Command{Use: name, Short: fmt.Sprintf("%s commands", name)}
	parent.AddCommand(cmd)
	return cmd
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

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func rawParentName(path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) == 0 {
		return "root"
	}
	last := segments[len(segments)-1]
	if last == "" {
		return "root"
	}
	return normalizeSegment(last)
}

func (p *Parser) createOperationCommand(use, method, path string, op *Operation, mapped *MappedCommand) *cobra.Command {
	short := strings.TrimSpace(op.Summary)
	if mapped != nil && strings.TrimSpace(mapped.Short) != "" {
		short = strings.TrimSpace(mapped.Short)
	}
	if short == "" {
		short = fmt.Sprintf("%s %s", method, path)
	}

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  buildOperationHelp(method, path, op, mapped),
		RunE: func(cmd *cobra.Command, args []string) error {
			if mapped != nil {
				cmd.SetContext(context.WithValue(cmd.Context(), mappedCommandContextKey{}, mapped))
			}
			showRollback, err := cmd.Flags().GetBool("rollback-plan")
			if err != nil {
				return err
			}
			if showRollback {
				return renderRollbackPlan(cmd, method, path, op, mapped)
			}
			showSchema, err := cmd.Flags().GetBool("schema")
			if err != nil {
				return err
			}
			if showSchema {
				return renderSchema(cmd, method, path, op, mapped)
			}
			return p.executeCommand(cmd, method, path, op)
		},
	}

	for _, param := range op.Parameters {
		if shouldSkipManagedParam(param) {
			continue
		}
		addParameterFlag(cmd, param, mappedFlagForParam(mapped, param))
	}
	if op.RequestBody != nil {
		cmd.Flags().String("body", "", "Request body as JSON string. Use this for complex or generated JSON input.")
		cmd.Flags().String("body-file", "", "Path to a JSON file used as request body. Takes precedence over --body.")
	}
	cmd.Flags().StringArray("query", nil, "Additional query parameter in key=value form. Repeat for multiple filters or dynamic API query keys.")
	cmd.Flags().Bool("schema", false, "Print request schema, enum hints, and examples without sending a request")
	cmd.Flags().Bool("rollback-plan", false, "Print rollback guidance for this write operation without sending a request")
	if isDangerousOperation(method, mapped) {
		cmd.Flags().Bool("yes", false, "Confirm this write operation")
	}
	addSemanticFlags(cmd, mapped)

	return cmd
}

func isDangerousOperation(method string, mapped *MappedCommand) bool {
	method = strings.ToUpper(method)
	if method == "DELETE" {
		return true
	}
	if mapped == nil {
		return false
	}
	path := strings.Join(mapped.Path, " ")
	return method == "PUT" && (strings.Contains(path, " enable") || strings.Contains(path, " disable") || strings.Contains(path, "delete"))
}

func addSemanticFlags(cmd *cobra.Command, mapped *MappedCommand) {
	if mapped == nil {
		return
	}
	switch strings.Join(mapped.Path, " ") {
	case "asset split create":
		cmd.Flags().String("api-id", "", "Original API UUID to split")
		cmd.Flags().String("origin", "query", "Split source (query|body|1|4)")
		cmd.Flags().String("key", "", "Predicate key to split by")
		cmd.Flags().String("name", "", "Predicate split rule name")
		cmd.Flags().String("comment", "", "Predicate split rule comment")
		cmd.Flags().Bool("disabled", false, "Create the rule disabled")
	case "risk strategy create-account-abuse-ip":
		cmd.Flags().String("scope", "site", "Effective scope (api|site|app|global|1|2|3|4)")
		cmd.Flags().StringArray("uuid", nil, "Target UUID. Repeat for multiple targets.")
		cmd.Flags().String("name", "账号滥用 IP", "Risk strategy name")
		cmd.Flags().Int("src-ip-dis-cnt", 10, "Distinct source IP count threshold")
		cmd.Flags().Int("detection-cycle", 2, "Detection cycle")
		cmd.Flags().Bool("disabled", false, "Create the strategy disabled")
	}
}

func buildOperationHelp(method, path string, op *Operation, mapped *MappedCommand) string {
	var b strings.Builder
	if mapped != nil && strings.TrimSpace(mapped.Long) != "" {
		b.WriteString(strings.TrimSpace(mapped.Long))
		b.WriteString("\n\n")
	} else if strings.TrimSpace(op.Description) != "" {
		b.WriteString(strings.TrimSpace(op.Description))
		b.WriteString("\n\n")
	}
	fmt.Fprintf(&b, "Endpoint: %s %s\n", method, path)
	if op.OperationID != "" {
		fmt.Fprintf(&b, "Operation ID: %s\n", op.OperationID)
	}
	if len(op.Parameters) > 0 {
		b.WriteString("\nParameters:\n")
		for _, param := range op.Parameters {
			if shouldSkipManagedParam(param) {
				continue
			}
			required := "optional"
			if param.Required {
				required = "required"
			}
			desc := strings.TrimSpace(param.Description)
			if desc == "" {
				desc = param.Name
			}
			fmt.Fprintf(&b, "- --%s (%s, %s): %s\n", flagNameForParam(param), param.In, required, desc)
		}
	}
	if op.RequestBody != nil {
		b.WriteString("\nBody:\n")
		b.WriteString("- Use --body for inline JSON or --body-file for a JSON file. --body-file takes precedence.\n")
		b.WriteString("- Use --schema to print request schema, enum hints, and examples without sending a request.\n")
		if op.RequestBody.Required {
			b.WriteString("- Request body is required by the API schema.\n")
		}
		if schema := requestJSONSchema(op.RequestBody); schema != nil {
			b.WriteString("\nRequest schema:\n")
			writeSchemaHelp(&b, schema, mapped, "")
		}
	}
	if mapped != nil && len(mapped.Aliases) > 0 {
		b.WriteString("\nProduct concepts:\n")
		for _, alias := range mapped.Aliases {
			fmt.Fprintf(&b, "- %s\n", alias)
		}
	}
	if mapped != nil && len(mapped.BodyExample) > 0 {
		b.WriteString("\nExample body:\n")
		if data, err := marshalPretty(mapped.BodyExample); err == nil {
			b.WriteString(string(data))
			b.WriteString("\n")
		}
	}
	if mapped != nil && mapped.Rollback != nil {
		b.WriteString("\nRollback:\n")
		if mapped.Rollback.Description != "" {
			fmt.Fprintf(&b, "- %s\n", mapped.Rollback.Description)
		}
		if mapped.Rollback.Command != "" {
			fmt.Fprintf(&b, "  %s\n", mapped.Rollback.Command)
		}
	}
	if mapped != nil && len(mapped.Query) > 0 {
		b.WriteString("\nDefault query:\n")
		for _, key := range sortedStringKeys(mapped.Query) {
			fmt.Fprintf(&b, "- %s=%s\n", key, mapped.Query[key])
		}
	}
	b.WriteString("\nDynamic query filters:\n")
	b.WriteString("- Use --query key=value for query keys not listed as dedicated flags. Repeat --query for multiple values.\n")
	if mapped != nil && len(mapped.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, example := range mapped.Examples {
			fmt.Fprintf(&b, "  %s\n", example)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func requestJSONSchema(requestBody *RequestBody) *Schema {
	if requestBody == nil {
		return nil
	}
	media, ok := requestBody.Content["application/json"]
	if !ok {
		return nil
	}
	return media.Schema
}

func writeSchemaHelp(b *strings.Builder, schema *Schema, mapped *MappedCommand, prefix string) {
	if schema == nil {
		return
	}
	if schema.Type == "array" {
		fmt.Fprintf(b, "- %s: array\n", printableSchemaName(prefix, "body"))
		writeSchemaHelp(b, schema.Items, mapped, prefix+"[]")
		return
	}
	if len(schema.Properties) == 0 {
		return
	}
	required := make(map[string]bool)
	for _, name := range schema.Required {
		required[name] = true
	}
	for _, name := range sortedSchemaKeys(schema.Properties) {
		prop := schema.Properties[name]
		fieldName := name
		if prefix != "" {
			fieldName = prefix + "." + name
		}
		req := "optional"
		if required[name] {
			req = "required"
		}
		desc := strings.TrimSpace(prop.Description)
		if desc == "" {
			desc = name
		}
		fmt.Fprintf(b, "- %s (%s, %s): %s", fieldName, schemaType(prop), req, desc)
		if mapped != nil {
			if hints := mapped.EnumHints[name]; len(hints) > 0 {
				b.WriteString(". Values: ")
				parts := make([]string, 0, len(hints))
				for _, hint := range hints {
					parts = append(parts, hint.Label+"="+hint.Value)
				}
				b.WriteString(strings.Join(parts, ", "))
			}
		}
		b.WriteString("\n")
	}
}

func printableSchemaName(prefix, fallback string) string {
	if prefix == "" {
		return fallback
	}
	return prefix
}

func sortedSchemaKeys(values map[string]*Schema) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func renderSchema(cmd *cobra.Command, method, path string, op *Operation, mapped *MappedCommand) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Endpoint: %s %s\n", method, path)
	if op.OperationID != "" {
		fmt.Fprintf(&b, "Operation ID: %s\n", op.OperationID)
	}
	if schema := requestJSONSchema(op.RequestBody); schema != nil {
		b.WriteString("\nRequest schema:\n")
		writeSchemaHelp(&b, schema, mapped, "")
	}
	if mapped != nil && len(mapped.BodyExample) > 0 {
		b.WriteString("\nExample body:\n")
		if data, err := marshalPretty(mapped.BodyExample); err == nil {
			b.WriteString(string(data))
			b.WriteString("\n")
		}
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(b.String(), "\n"))
	return err
}

func marshalPretty(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func renderRollbackPlan(cmd *cobra.Command, method, path string, op *Operation, mapped *MappedCommand) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Will execute: %s %s\n", method, path)
	if op.OperationID != "" {
		fmt.Fprintf(&b, "Operation ID: %s\n", op.OperationID)
	}
	if mapped != nil && mapped.Rollback != nil {
		b.WriteString("\nRollback:\n")
		if mapped.Rollback.Description != "" {
			fmt.Fprintf(&b, "%s\n", mapped.Rollback.Description)
		}
		if mapped.Rollback.Command != "" {
			fmt.Fprintf(&b, "%s\n", mapped.Rollback.Command)
		}
	} else {
		b.WriteString("\nRollback: no product-specific rollback plan is available for this operation.\n")
	}
	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(b.String(), "\n"))
	return err
}

func addParameterFlag(cmd *cobra.Command, param Parameter, mapped *MappedFlag) {
	name := flagNameForParam(param)
	if mapped != nil && mapped.Name != "" {
		name = normalizeSegment(mapped.Name)
	}
	if cmd.Flags().Lookup(name) != nil {
		return
	}
	desc := strings.TrimSpace(param.Description)
	if mapped != nil && strings.TrimSpace(mapped.Description) != "" {
		desc = strings.TrimSpace(mapped.Description)
	}
	if desc == "" {
		desc = fmt.Sprintf("%s %s parameter", param.In, param.Name)
	}
	switch schemaType(param.Schema) {
	case "integer":
		cmd.Flags().Int(name, 0, desc)
	case "boolean":
		cmd.Flags().Bool(name, false, desc)
	default:
		cmd.Flags().String(name, "", desc)
	}
	if param.Required {
		_ = cmd.MarkFlagRequired(name)
	}
}

func mappedFlagForParam(mapped *MappedCommand, param Parameter) *MappedFlag {
	if mapped == nil || len(mapped.Flags) == 0 {
		return nil
	}
	if flag, ok := mapped.Flags[param.Name]; ok {
		return &flag
	}
	if flag, ok := mapped.Flags[flagNameForParam(param)]; ok {
		return &flag
	}
	return nil
}

func (p *Parser) executeCommand(cmd *cobra.Command, method, path string, op *Operation) error {
	if err := requireConfirmation(cmd, method, mappedCommandFromContext(cmd)); err != nil {
		return err
	}
	query := make(url.Values)
	if mapped := mappedCommandFromContext(cmd); mapped != nil {
		for key, value := range mapped.Query {
			query.Set(key, value)
		}
	}
	body, err := collectRequestBody(cmd, op.RequestBody)
	if err != nil {
		return err
	}
	apiPath := path

	for _, param := range op.Parameters {
		if shouldSkipManagedParam(param) {
			continue
		}
		value, present, err := readParameterFlag(cmd, param)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		switch strings.ToLower(param.In) {
		case "path":
			apiPath = strings.ReplaceAll(apiPath, "{"+param.Name+"}", value)
		case "query":
			query.Set(param.Name, value)
		}
	}
	if err := collectDynamicQuery(cmd, query); err != nil {
		return err
	}

	client := getClient(cmd)
	var result any
	if err := client.Do(cmd.Context(), method, apiPath, query, body, &result); err != nil {
		if _, ok := err.(dryRunResult); ok {
			return nil
		}
		return err
	}
	if wrapped, ok := wrapMutationResult(method, result, mappedCommandFromContext(cmd)); ok {
		return getRenderer(cmd).Render(wrapped)
	}
	return getRenderer(cmd).Render(result)
}

func requireConfirmation(cmd *cobra.Command, method string, mapped *MappedCommand) error {
	if !isDangerousOperation(method, mapped) {
		return nil
	}
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}
	if yes {
		return nil
	}
	return fmt.Errorf("this operation can modify or delete resources; pass --yes to confirm")
}

func wrapMutationResult(method string, result any, mapped *MappedCommand) (map[string]any, bool) {
	if mapped == nil || mapped.Rollback == nil || strings.ToUpper(method) != "POST" {
		return nil, false
	}
	id := extractResultID(result)
	rollback := mapped.Rollback.Command
	if id != "" {
		rollback = strings.ReplaceAll(rollback, "<created_id>", id)
	}
	wrapped := map[string]any{
		"action":   "created",
		"resource": resourceName(mapped),
		"result":   result,
		"rollback": rollback,
	}
	if id != "" {
		wrapped["id"] = id
	}
	return wrapped, true
}

func resourceName(mapped *MappedCommand) string {
	if mapped == nil {
		return "unknown"
	}
	if len(mapped.Path) > 1 {
		return strings.Join(mapped.Path[:len(mapped.Path)-1], ":")
	}
	if len(mapped.Path) == 1 {
		return mapped.Path[0]
	}
	if mapped.OperationID != "" {
		return mapped.OperationID
	}
	return "unknown"
}

func extractResultID(result any) string {
	if result == nil {
		return ""
	}
	switch values := result.(type) {
	case map[string]any:
		for _, key := range []string{"id", "uuid"} {
			if value, ok := values[key]; ok && value != nil {
				return fmt.Sprint(value)
			}
		}
		for _, value := range values {
			if id := extractResultID(value); id != "" {
				return id
			}
		}
	case []any:
		for _, value := range values {
			if id := extractResultID(value); id != "" {
				return id
			}
		}
	}
	return ""
}

func mappedCommandFromContext(cmd *cobra.Command) *MappedCommand {
	value := cmd.Context().Value(mappedCommandContextKey{})
	mapped, _ := value.(*MappedCommand)
	return mapped
}

type mappedCommandContextKey struct{}

func collectDynamicQuery(cmd *cobra.Command, query url.Values) error {
	values, err := cmd.Flags().GetStringArray("query")
	if err != nil {
		return err
	}
	for _, item := range values {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return fmt.Errorf("invalid --query %q, want key=value", item)
		}
		query.Add(key, value)
	}
	return nil
}

func readParameterFlag(cmd *cobra.Command, param Parameter) (string, bool, error) {
	flag := cmd.Flags().Lookup(flagNameForParam(param))
	if flag == nil || !flag.Changed {
		return "", false, nil
	}
	switch schemaType(param.Schema) {
	case "integer":
		value, err := cmd.Flags().GetInt(flag.Name)
		if err != nil {
			return "", false, err
		}
		return fmt.Sprintf("%d", value), true, nil
	case "boolean":
		value, err := cmd.Flags().GetBool(flag.Name)
		if err != nil {
			return "", false, err
		}
		return fmt.Sprintf("%t", value), true, nil
	default:
		value, err := cmd.Flags().GetString(flag.Name)
		return value, true, err
	}
}

func collectRequestBody(cmd *cobra.Command, requestBody *RequestBody) (any, error) {
	if requestBody == nil {
		return nil, nil
	}
	bodyFile, err := cmd.Flags().GetString("body-file")
	if err != nil {
		return nil, err
	}
	if bodyFile != "" {
		data, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("read body file %s: %w", bodyFile, err)
		}
		return parseJSONBody(data)
	}
	bodyText, err := cmd.Flags().GetString("body")
	if err != nil {
		return nil, err
	}
	if bodyText == "" {
		if body, ok, err := collectSemanticBody(cmd); ok || err != nil {
			return body, err
		}
		return nil, nil
	}
	return parseJSONBody([]byte(bodyText))
}

func collectSemanticBody(cmd *cobra.Command) (any, bool, error) {
	mapped := mappedCommandFromContext(cmd)
	if mapped == nil {
		return nil, false, nil
	}
	switch strings.Join(mapped.Path, " ") {
	case "asset split create":
		apiID, _ := cmd.Flags().GetString("api-id")
		key, _ := cmd.Flags().GetString("key")
		name, _ := cmd.Flags().GetString("name")
		comment, _ := cmd.Flags().GetString("comment")
		origin, _ := cmd.Flags().GetString("origin")
		disabled, _ := cmd.Flags().GetBool("disabled")
		if apiID == "" && key == "" && name == "" {
			return nil, false, nil
		}
		if apiID == "" || key == "" || name == "" {
			return nil, true, fmt.Errorf("--api-id, --key, and --name are required when using split create flags")
		}
		body := map[string]any{
			"is_enabled":      !disabled,
			"name":            name,
			"origin":          normalizeSplitOrigin(origin),
			"key":             key,
			"original_api_id": apiID,
		}
		if comment != "" {
			body["comment"] = comment
		}
		return body, true, nil
	case "risk strategy create-account-abuse-ip":
		if !cmd.Flags().Changed("uuid") && !cmd.Flags().Changed("src-ip-dis-cnt") && !cmd.Flags().Changed("detection-cycle") && !cmd.Flags().Changed("scope") {
			return nil, false, nil
		}
		scope, _ := cmd.Flags().GetString("scope")
		uuids, _ := cmd.Flags().GetStringArray("uuid")
		name, _ := cmd.Flags().GetString("name")
		srcIPDisCnt, _ := cmd.Flags().GetInt("src-ip-dis-cnt")
		detectionCycle, _ := cmd.Flags().GetInt("detection-cycle")
		disabled, _ := cmd.Flags().GetBool("disabled")
		body := map[string]any{
			"effective_scope": normalizeEffectiveScope(scope),
			"uuids":           uuids,
			"name":            name,
			"type":            "55",
			"is_enabled":      !disabled,
			"pattern": map[string]any{
				"detection_cycle": detectionCycle,
				"src_ip_dis_cnt":  srcIPDisCnt,
				"business_tag":    []any{},
			},
		}
		return body, true, nil
	default:
		return nil, false, nil
	}
}

func normalizeSplitOrigin(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "query", "1":
		return "1"
	case "body", "4":
		return "4"
	default:
		return value
	}
}

func normalizeEffectiveScope(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "api", "1":
		return "1"
	case "site", "2":
		return "2"
	case "app", "3":
		return "3"
	case "global", "4":
		return "4"
	default:
		return value
	}
}

func parseJSONBody(data []byte) (any, error) {
	var body any
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("parse request body JSON: %w", err)
	}
	return body, nil
}

func getClient(cmd *cobra.Command) *Client {
	urlValue, _ := cmd.Flags().GetString("url")
	apiToken, _ := cmd.Flags().GetString("api-token")
	return NewClient(&Config{URL: urlValue, APIToken: apiToken}, verbose)
}

func getRenderer(cmd *cobra.Command) Renderer {
	format := FormatJSON
	if output, _ := cmd.Flags().GetString("output"); output == string(FormatTable) {
		format = FormatTable
	}
	return NewRenderer(format, cmd.OutOrStdout())
}

func shouldSkipManagedParam(param Parameter) bool {
	if strings.ToLower(param.In) != "header" {
		return false
	}
	switch strings.ToLower(param.Name) {
	case "api-token", "content-type", "accept":
		return true
	default:
		return false
	}
}

func flagNameForParam(param Parameter) string {
	return normalizeSegment(param.Name)
}

func schemaType(schema *Schema) string {
	if schema == nil || schema.Type == "" {
		return "string"
	}
	return schema.Type
}

var nonAlphaNumeric = regexp.MustCompile(`[^a-z0-9-]+`)

func normalizeSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "{}")
	var b strings.Builder
	var prev rune
	for i, r := range value {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev) || (prev == 'I' && r == 'A')) {
			b.WriteRune('-')
		}
		switch r {
		case '_', '.', '/', ' ':
			b.WriteRune('-')
		default:
			b.WriteRune(unicode.ToLower(r))
		}
		prev = r
	}
	result := nonAlphaNumeric.ReplaceAllString(b.String(), "-")
	result = strings.Trim(result, "-")
	result = strings.ReplaceAll(result, "a-p-i", "api")
	return result
}
