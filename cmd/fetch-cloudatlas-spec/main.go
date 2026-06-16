package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Spec struct {
	OpenAPI    string                `yaml:"openapi"`
	Info       map[string]any        `yaml:"info"`
	Paths      map[string]PathItem   `yaml:"paths"`
	Components map[string]any        `yaml:"components,omitempty"`
	Security   []map[string][]string `yaml:"security,omitempty"`
}

type PathItem map[string]any

func main() {
	input := flag.String("input", "", "URL list file, defaults to stdin")
	output := flag.String("output", "products/cloudatlas/spec/openapi.yaml", "Output merged OpenAPI YAML path")
	flag.Parse()

	urls, err := readURLs(*input)
	if err != nil {
		fatal(err)
	}
	if len(urls) == 0 {
		fatal(fmt.Errorf("no URLs provided"))
	}

	cookie := os.Getenv("CLOUD_ATLAS_APIFOX_COOKIE")
	if cookie == "" {
		fatal(fmt.Errorf("CLOUD_ATLAS_APIFOX_COOKIE is required"))
	}

	merged := Spec{
		OpenAPI: "3.0.1",
		Info: map[string]any{
			"title":       "Cloud Atlas OpenAPI",
			"description": "Merged from Apifox shared markdown documents",
			"version":     "1.0.0",
		},
		Paths: map[string]PathItem{},
		Components: map[string]any{
			"schemas": map[string]any{},
			"securitySchemes": map[string]any{
				"apikey-header-TOKEN": map[string]any{"type": "apiKey", "in": "header", "name": "TOKEN"},
			},
		},
		Security: []map[string][]string{{"apikey-header-TOKEN": {}}},
	}

	client := &http.Client{Timeout: 30 * time.Second}
	for _, rawURL := range urls {
		markdown, err := fetch(client, rawURL, cookie)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: fetch %s: %v\n", rawURL, err)
			continue
		}
		specText, err := extractOpenAPI(markdown)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: extract %s: %v\n", rawURL, err)
			continue
		}
		var spec Spec
		if err := yaml.Unmarshal([]byte(specText), &spec); err != nil {
			fmt.Fprintf(os.Stderr, "warning: parse %s: %v\n", rawURL, err)
			continue
		}
		mergeSpec(&merged, &spec, rawURL)
	}

	if len(merged.Paths) == 0 {
		fatal(fmt.Errorf("no OpenAPI paths merged"))
	}

	data, err := marshalStable(merged)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fatal(err)
	}
	fmt.Fprintf(os.Stderr, "merged %d paths into %s\n", len(merged.Paths), *output)
}

func readURLs(path string) ([]string, error) {
	var r io.Reader = os.Stdin
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}

	var urls []string
	scanner := bufio.NewScanner(r)
	linkPattern := regexp.MustCompile(`https://[^\s)]+`)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matches := linkPattern.FindAllString(line, -1)
		if len(matches) == 0 && strings.HasPrefix(line, "http") {
			urls = append(urls, line)
			continue
		}
		urls = append(urls, matches...)
	}
	return urls, scanner.Err()
}

func fetch(client *http.Client, rawURL, cookie string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", "chaitin-cli-cloud-atlas-spec-fetcher")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return string(body), nil
}

func extractOpenAPI(markdown string) (string, error) {
	pattern := regexp.MustCompile("(?s)```ya?ml\\s*(.*?)```")
	for _, match := range pattern.FindAllStringSubmatch(markdown, -1) {
		if strings.Contains(match[1], "openapi:") && strings.Contains(match[1], "paths:") {
			return strings.TrimSpace(match[1]) + "\n", nil
		}
	}
	return "", fmt.Errorf("OpenAPI YAML block not found")
}

func mergeSpec(dst, src *Spec, source string) {
	for path, pathItem := range src.Paths {
		if dst.Paths[path] == nil {
			dst.Paths[path] = PathItem{}
		}
		for method, operation := range pathItem {
			method = strings.ToLower(method)
			if _, exists := dst.Paths[path][method]; exists {
				fmt.Fprintf(os.Stderr, "warning: duplicate %s %s from %s ignored\n", strings.ToUpper(method), path, source)
				continue
			}
			dst.Paths[path][method] = operation
		}
	}
}

func marshalStable(spec Spec) ([]byte, error) {
	root := yaml.Node{Kind: yaml.MappingNode}
	appendScalarMap(&root, "openapi", spec.OpenAPI)
	appendAny(&root, "info", spec.Info)

	pathsNode := yaml.Node{Kind: yaml.MappingNode}
	paths := make([]string, 0, len(spec.Paths))
	for path := range spec.Paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		appendAny(&pathsNode, path, map[string]any(spec.Paths[path]))
	}
	root.Content = append(root.Content, scalarNode("paths"), &pathsNode)
	appendAny(&root, "components", spec.Components)
	appendAny(&root, "security", spec.Security)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func appendScalarMap(node *yaml.Node, key, value string) {
	node.Content = append(node.Content, scalarNode(key), scalarNode(value))
}

func appendAny(node *yaml.Node, key string, value any) {
	var valueNode yaml.Node
	_ = valueNode.Encode(value)
	node.Content = append(node.Content, scalarNode(key), &valueNode)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
