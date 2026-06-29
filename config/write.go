package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func SetProduct(path, name string, value any) error {
	cfg, err := Load(path)
	if err != nil {
		return fmt.Errorf("%w; use -c/--config to specify a writable config file", err)
	}

	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return fmt.Errorf("encode config for %s: %w", name, err)
	}
	cfg[name] = node

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config file %s: %w", path, err)
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create config dir %s: %w; use -c/--config to specify a writable config file", dir, err)
		}
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file %s: %w; use -c/--config to specify a writable config file", path, err)
	}

	return nil
}
