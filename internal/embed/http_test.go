package embed_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agmemx/internal/embed"
)

func TestEffectiveBase(t *testing.T) {
	if got := embed.EffectiveBase("ollama", ""); got != "http://127.0.0.1:11434" {
		t.Fatalf("ollama default %s", got)
	}
	if got := embed.EffectiveBase("openai", ""); got != "https://api.openai.com/v1" {
		t.Fatalf("openai default %s", got)
	}
	if got := embed.EffectiveBase("fixture", ""); got != "" {
		t.Fatalf("fixture default %q", got)
	}
	if got := embed.EffectiveBase("openai", "https://api.x.ai/v1/"); got != "https://api.x.ai/v1" {
		t.Fatalf("trimmed %s", got)
	}
}

func TestClientTimeoutIsTenSeconds(t *testing.T) {
	if embed.Client.Timeout != 10*time.Second {
		t.Fatalf("timeout %s", embed.Client.Timeout)
	}
}

func TestOllamaAndOpenAIRequests(t *testing.T) {
	ollama := map[string]string{}
	var ollamaBody []byte
	ollamaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ollama["method"] = r.Method
		ollama["path"] = r.URL.Path
		if _, ok := r.Header["Authorization"]; ok {
			t.Errorf("ollama set Authorization %q", r.Header.Get("Authorization"))
		}
		ollama["type"] = r.Header.Get("Content-Type")
		var err error
		ollamaBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"nomic-embed-text","embeddings":[[0.25,0.5]],"total_duration":1}`)
	}))
	defer ollamaSrv.Close()

	vec, err := embed.Ollama(ollamaSrv.URL+"/", "nomic-embed-text", "発話の記録")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 2 || vec[0] != 0.25 || vec[1] != 0.5 {
		t.Fatalf("ollama vector %v", vec)
	}
	if ollama["method"] != http.MethodPost || ollama["path"] != "/api/embed" {
		t.Fatalf("ollama request %#v", ollama)
	}
	if ollama["type"] != "application/json" {
		t.Fatalf("content-type %s", ollama["type"])
	}
	if string(ollamaBody) != `{"model":"nomic-embed-text","input":"発話の記録"}` {
		t.Fatalf("ollama body %s", ollamaBody)
	}

	openai := map[string]string{}
	var openaiBody []byte
	openaiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openai["method"] = r.Method
		openai["path"] = r.URL.Path
		openai["auth"] = r.Header.Get("Authorization")
		var err error
		openaiBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		_, _ = io.WriteString(w, `{"object":"list","data":[{"object":"embedding","index":0,"embedding":[1,0,0.5]}],"model":"text-embedding-3-small","usage":{"prompt_tokens":1,"total_tokens":1}}`)
	}))
	defer openaiSrv.Close()

	vec, err = embed.OpenAI(openaiSrv.URL, "text-embedding-3-small", "発話の記録", "sk-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(vec) != 3 || vec[0] != 1 || vec[1] != 0 || vec[2] != 0.5 {
		t.Fatalf("openai vector %v", vec)
	}
	if openai["method"] != http.MethodPost || openai["path"] != "/embeddings" {
		t.Fatalf("openai request %#v", openai)
	}
	if openai["auth"] != "Bearer sk-test" {
		t.Fatalf("auth %q", openai["auth"])
	}
	if string(openaiBody) != `{"model":"text-embedding-3-small","input":"発話の記録"}` {
		t.Fatalf("openai body %s", openaiBody)
	}
	assertObject(t, ollamaBody)
	assertObject(t, openaiBody)
}

func TestMissingVectorIsRejected(t *testing.T) {
	bodies := []string{
		`{}`,
		`{"embeddings":[]}`,
		`{"embeddings":[[]]}`,
		`{"embeddings":[null]}`,
		`not-json`,
		`{"data":[]}`,
		`{"data":[{"embedding":null}]}`,
		`{"data":[{"embedding":[]}]}`,
	}
	for _, body := range bodies {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, body)
		}))
		_, err := embed.Ollama(srv.URL, "m", "t")
		if !errors.Is(err, embed.ErrRejected) {
			t.Fatalf("ollama body %s: %v", body, err)
		}
		_, err = embed.OpenAI(srv.URL, "m", "t", "sk")
		if !errors.Is(err, embed.ErrRejected) {
			t.Fatalf("openai body %s: %v", body, err)
		}
		srv.Close()
	}
	status := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"nope"}`, http.StatusUnauthorized)
	}))
	defer status.Close()
	if _, err := embed.Ollama(status.URL, "m", "t"); !errors.Is(err, embed.ErrRejected) {
		t.Fatalf("ollama status %v", err)
	}
	if _, err := embed.OpenAI(status.URL, "m", "t", "sk"); !errors.Is(err, embed.ErrRejected) {
		t.Fatal(err)
	}
}

func TestTimeoutAndRefusedAreUnreachable(t *testing.T) {
	hung := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
	}))
	defer hung.Close()
	prev := embed.Client
	embed.Client = &http.Client{Timeout: 100 * time.Millisecond}
	t.Cleanup(func() { embed.Client = prev })
	_, err := embed.Ollama(hung.URL, "m", "t")
	if !errors.Is(err, embed.ErrUnreachable) {
		t.Fatalf("timeout %v", err)
	}

	refused := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := refused.URL
	refused.Close()
	embed.Client = prev
	_, err = embed.OpenAI(url, "m", "t", "sk")
	if !errors.Is(err, embed.ErrUnreachable) {
		t.Fatalf("refused %v", err)
	}
}

func assertObject(t *testing.T, body []byte) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	var fields map[string]string
	if err := dec.Decode(&fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields["model"] == "" || fields["input"] == "" {
		t.Fatalf("fields %#v", fields)
	}
}
