package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultEmbedProvider = "ollama"
	defaultOllamaModel   = "nomic-embed-text"
)

type fileConfig struct {
	EmbedProvider string `json:"embed_provider,omitempty"`
	EmbedModel    string `json:"embed_model,omitempty"`
	EmbedBaseURL  string `json:"embed_base_url,omitempty"`
	EmbedAPIKey   string `json:"embed_api_key_env,omitempty"`
}

func configPath(environ []string) (string, *rejection) {
	env := map[string]string{}
	for _, pair := range environ {
		key, val, ok := strings.Cut(pair, "=")
		if ok {
			env[key] = val
		}
	}
	base, ok := env["XDG_CONFIG_HOME"]
	if !ok || base == "" {
		home := env["HOME"]
		base = filepath.Join(home, ".config")
	}
	if !filepath.IsAbs(base) {
		return "", reject("xdg_relative", 1)
	}
	return filepath.Join(base, "agmemx", "config.json"), nil
}

func loadFileConfig(environ []string) (fileConfig, *rejection) {
	path, rej := configPath(environ)
	if rej != nil {
		return fileConfig{}, rej
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileConfig{}, nil
	}
	if err != nil {
		return fileConfig{}, reject("invalid_json", 2)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var cfg fileConfig
	if err := dec.Decode(&cfg); err != nil {
		return fileConfig{}, reject("invalid_json", 2)
	}
	return cfg, nil
}

func (opt *options) applyEmbed(cfg fileConfig) {
	if opt.provider == "" {
		if cfg.EmbedProvider != "" {
			opt.provider = cfg.EmbedProvider
		} else {
			opt.provider = defaultEmbedProvider
		}
	}
	if opt.model == "" {
		if cfg.EmbedModel != "" {
			opt.model = cfg.EmbedModel
		} else if opt.provider == defaultEmbedProvider {
			opt.model = defaultOllamaModel
		}
	}
	if opt.baseURL == "" {
		opt.baseURL = cfg.EmbedBaseURL
	}
	if opt.keyEnv == "" {
		opt.keyEnv = cfg.EmbedAPIKey
	}
}

func needsEmbed(command string) bool {
	switch command {
	case "observe", "believe", "search", "reindex":
		return true
	default:
		return false
	}
}

func handleConfig(stdout, stderr io.Writer, opts options, environ []string) int {
	args := opts.positionals
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch args[0] {
	case "path":
		path, rej := configPath(environ)
		if rej != nil {
			writeReject(stdout, rej)
			return rej.exit
		}
		fmt.Fprintln(stdout, path)
		return 0
	case "show":
		return showConfig(stdout, environ)
	case "set":
		if len(args) != 3 {
			writeReject(stdout, reject("missing_field", 2))
			return 2
		}
		return writeConfigKey(stdout, environ, args[1], args[2])
	case "unset":
		if len(args) != 2 {
			writeReject(stdout, reject("missing_field", 2))
			return 2
		}
		return writeConfigKey(stdout, environ, args[1], "")
	default:
		writeReject(stdout, reject("invalid_command", 2))
		return 2
	}
}

func showConfig(stdout io.Writer, environ []string) int {
	path, rej := configPath(environ)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	cfg, rej := loadFileConfig(environ)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	provider, providerSource := effective(cfg.EmbedProvider, defaultEmbedProvider)
	model := cfg.EmbedModel
	modelSource := "config"
	if model == "" && provider == defaultEmbedProvider {
		model = defaultOllamaModel
		modelSource = "default"
	} else if model == "" {
		modelSource = "unset"
	}
	fmt.Fprintf(stdout, "path %s\n", path)
	fmt.Fprintf(stdout, "embed-provider %s %s\n", provider, sourceLabel(cfg.EmbedProvider, providerSource))
	fmt.Fprintf(stdout, "embed-model %s %s\n", model, modelSource)
	fmt.Fprintf(stdout, "embed-base-url %s\n", cfg.EmbedBaseURL)
	fmt.Fprintf(stdout, "embed-api-key-env %s\n", cfg.EmbedAPIKey)
	return 0
}

func sourceLabel(explicit, fallback string) string {
	if explicit != "" {
		return "config"
	}
	return fallback
}

func effective(explicit, fallback string) (string, string) {
	if explicit != "" {
		return explicit, "config"
	}
	return fallback, "default"
}

func writeConfigKey(stdout io.Writer, environ []string, key, value string) int {
	field, ok := configFields[key]
	if !ok {
		writeReject(stdout, reject("invalid_flag", 2))
		return 2
	}
	if key == "embed-provider" && value != "" && value != "ollama" && value != "openai" && value != "fixture" {
		writeReject(stdout, reject("invalid_flag", 2))
		return 2
	}
	path, rej := configPath(environ)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	cfg, rej := loadFileConfig(environ)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	switch field {
	case "embed_provider":
		cfg.EmbedProvider = value
	case "embed_model":
		cfg.EmbedModel = value
	case "embed_base_url":
		cfg.EmbedBaseURL = value
	case "embed_api_key_env":
		cfg.EmbedAPIKey = value
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		fmt.Fprintf(stdout, "agmemx: %v\n", err)
		return 2
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		writeReject(stdout, reject("invalid_json", 2))
		return 2
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o600); err != nil {
		fmt.Fprintf(stdout, "agmemx: %v\n", err)
		return 2
	}
	fmt.Fprintf(stdout, "%s %s\n", key, value)
	return 0
}

var configFields = map[string]string{
	"embed-provider":    "embed_provider",
	"embed-model":       "embed_model",
	"embed-base-url":    "embed_base_url",
	"embed-api-key-env": "embed_api_key_env",
}
