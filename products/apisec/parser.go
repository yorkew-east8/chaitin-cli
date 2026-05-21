package apisec

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
)

type Parser struct {
	api     *OpenAPI
	mapping *CLIMapping
}

type operationRef struct {
	method string
	path   string
	op     *Operation
}

func NewParser(api *OpenAPI, mapping *CLIMapping) *Parser {
	return &Parser{api: api, mapping: mapping}
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
			parent.AddCommand(p.createOperationCommand(childName, item.method, path, item.op, nil))
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

	return cmd
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
		if op.RequestBody.Required {
			b.WriteString("- Request body is required by the API schema.\n")
		}
	}
	if mapped != nil && len(mapped.Examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, example := range mapped.Examples {
			fmt.Fprintf(&b, "  %s\n", example)
		}
	}
	return strings.TrimRight(b.String(), "\n")
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
	query := make(url.Values)
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

	client := getClient(cmd)
	var result any
	if err := client.Do(cmd.Context(), method, apiPath, query, body, &result); err != nil {
		return err
	}
	return getRenderer(cmd).Render(result)
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
		return nil, nil
	}
	return parseJSONBody([]byte(bodyText))
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
