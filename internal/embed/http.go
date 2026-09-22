package embed

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultOllamaBase is the base URL used when --embed-base-url is omitted.
	DefaultOllamaBase = "http://127.0.0.1:11434"
	// DefaultOpenAIBase is the base URL used when --embed-base-url is omitted.
	DefaultOpenAIBase = "https://api.openai.com/v1"
)

// ErrUnreachable means the provider could not be contacted within 10 seconds.
var ErrUnreachable = errors.New("embed unreachable")

// ErrRejected means the provider responded without a vector.
var ErrRejected = errors.New("embed rejected")

// Client issues hosted embedding requests. The timeout is the contract limit.
var Client = &http.Client{Timeout: 10 * time.Second}

// EffectiveBase is the base URL stored in the cache key and used for requests.
// An empty configured URL selects the provider default. Fixture has no default.
func EffectiveBase(provider, configured string) string {
	if configured != "" {
		return strings.TrimRight(configured, "/")
	}
	switch provider {
	case "ollama":
		return DefaultOllamaBase
	case "openai":
		return DefaultOpenAIBase
	default:
		return ""
	}
}

// Ollama posts {base}/api/embed and returns embeddings[0].
func Ollama(baseURL, model, text string) ([]float64, error) {
	return post(join(baseURL, "/api/embed"), model, text, "", parseOllama)
}

// OpenAI posts {base}/embeddings and returns data[0].embedding.
func OpenAI(baseURL, model, text, apiKey string) ([]float64, error) {
	return post(join(baseURL, "/embeddings"), model, text, apiKey, parseOpenAI)
}

func join(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + path
}

func post(endpoint, model, text, apiKey string, parse func([]byte) ([]float64, error)) ([]float64, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	body, err := marshalInput(model, text)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrRejected
	}
	return parse(raw)
}

func marshalInput(model, text string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(struct {
		Model string `json:"model"`
		Input string `json:"input"`
	}{Model: model, Input: text}); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func parseOllama(raw []byte) ([]float64, error) {
	var decoded struct {
		Embeddings [][]float64 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, ErrRejected
	}
	if len(decoded.Embeddings) == 0 || len(decoded.Embeddings[0]) == 0 {
		return nil, ErrRejected
	}
	return decoded.Embeddings[0], nil
}

func parseOpenAI(raw []byte) ([]float64, error) {
	var decoded struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, ErrRejected
	}
	if len(decoded.Data) == 0 || len(decoded.Data[0].Embedding) == 0 {
		return nil, ErrRejected
	}
	return decoded.Data[0].Embedding, nil
}
