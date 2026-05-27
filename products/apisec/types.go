package apisec

import "encoding/json"

type Config struct {
	URL      string `yaml:"url"`
	APIToken string `yaml:"api_token"`
}

type OpenAPI struct {
	OpenAPI string              `json:"openapi"`
	Info    Info                `json:"info"`
	Tags    []Tag               `json:"tags,omitempty"`
	Paths   map[string]PathItem `json:"paths"`
}

type Info struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

type Operation struct {
	OperationID string       `json:"operationId"`
	Summary     string       `json:"summary,omitempty"`
	Description string       `json:"description,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
	Parameters  []Parameter  `json:"parameters,omitempty"`
	RequestBody *RequestBody `json:"requestBody,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

type Schema struct {
	Type        string             `json:"type,omitempty"`
	Ref         string             `json:"$ref,omitempty"`
	Description string             `json:"description,omitempty"`
	Enum        []json.RawMessage  `json:"enum,omitempty"`
	Items       *Schema            `json:"items,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
}

type RequestBody struct {
	Required bool                 `json:"required,omitempty"`
	Content  map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema  *Schema `json:"schema,omitempty"`
	Example any     `json:"example,omitempty"`
}

type CLIMapping struct {
	Commands []MappedCommand `yaml:"commands"`
}

type MappedCommand struct {
	Path        []string               `yaml:"path"`
	OperationID string                 `yaml:"operationId"`
	Short       string                 `yaml:"short,omitempty"`
	Long        string                 `yaml:"long,omitempty"`
	Examples    []string               `yaml:"examples,omitempty"`
	Query       map[string]string      `yaml:"query,omitempty"`
	EnumHints   map[string][]EnumHint  `yaml:"enumHints,omitempty"`
	BodyExample map[string]interface{} `yaml:"bodyExample,omitempty"`
	Aliases     []string               `yaml:"aliases,omitempty"`
	Rollback    *RollbackHint          `yaml:"rollback,omitempty"`
	RawHidden   bool                   `yaml:"rawHidden,omitempty"`
	Flags       map[string]MappedFlag  `yaml:"flags,omitempty"`
	Metadata    map[string]interface{} `yaml:"metadata,omitempty"`
}

type EnumHint struct {
	Value string `yaml:"value"`
	Label string `yaml:"label"`
}

type RollbackHint struct {
	Description string `yaml:"description,omitempty"`
	Command     string `yaml:"command,omitempty"`
}

type MappedFlag struct {
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
}
