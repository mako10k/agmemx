// Package cli is the agmemx command surface defined by docs/cli-contract.md.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"agmemx/internal/domain"
	"agmemx/internal/store"
	"agmemx/internal/xdg"
)

// Run executes one invocation. cwd is the process directory captured at startup.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, environ []string, cwd string) int {
	opts, rej := parseArgs(args)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	roots, err := xdg.Resolve(environ)
	if err != nil {
		writeReject(stdout, reject("xdg_relative", 1))
		return 1
	}
	resolved, kind := domain.Resolve(cwd, opts.dir)
	switch kind {
	case domain.NotFound:
		writeReject(stdout, reject("dir_not_found", 1))
		return 1
	case domain.NotDirectory:
		writeReject(stdout, reject("dir_not_directory", 1))
		return 1
	case domain.IsRoot:
		writeReject(stdout, reject("dir_is_root", 1))
		return 1
	}
	body, rej := readObject(stdin)
	if rej != nil {
		writeReject(stdout, rej)
		return rej.exit
	}
	switch opts.command {
	case "init":
		if len(body) != 0 {
			writeReject(stdout, reject("unknown_field", 2))
			return 2
		}
		if err := store.InitDomain(roots.Data, resolved); err != nil {
			fmt.Fprintf(stderr, "agmemx: %v\n", err)
			return 2
		}
		writeOK(stdout, struct {
			Domain string `json:"domain"`
		}{Domain: resolved})
		return 0
	case "observe":
		return handleObserve(stdout, stderr, roots, opts, resolved, body)
	case "believe":
		return handleBelieve(stdout, stderr, roots, opts, resolved, body)
	case "search":
		return handleSearch(stdout, stderr, roots, opts, resolved, body)
	case "relate":
		return handleRelate(stdout, stderr, roots, opts, resolved, body)
	case "domain-attach":
		return handleDomainAttach(stdout, stderr, roots, opts, resolved, body)
	case "reindex":
		return handleReindex(stdout, stderr, roots, opts, resolved, body)
	default:
		writeReject(stdout, reject("invalid_command", 2))
		return 2
	}
}

type rejection struct {
	code    string
	message string
	exit    int
}

func reject(code string, exit int) *rejection {
	return &rejection{code: code, message: messages[code], exit: exit}
}

var messages = map[string]string{
	"invalid_json":                 "入力は1つの JSON オブジェクトである",
	"invalid_command":              "未知のコマンドである",
	"invalid_flag":                 "未知のフラグがある",
	"unknown_field":                "未知のフィールドがある",
	"missing_field":                "必須フィールドがない",
	"invalid_type":                 "フィールドの型が契約と違う",
	"xdg_relative":                 "XDG のパスが相対パスである",
	"dir_not_found":                "解決ディレクトリが存在しない",
	"dir_not_directory":            "解決先がディレクトリではない",
	"dir_is_root":                  "解決ディレクトリがファイルシステムのルートである",
	"text_empty":                   "本文が空である",
	"text_too_long":                "本文が8192文字を超える",
	"interval_invalid":             "期間の形式が契約と違う",
	"observation_reason_forbidden": "観測の理由は受け付けない",
	"reason_kind_invalid":          "信念の理由種別が契約にない",
	"reason_not_found":             "理由の対象が存在しない",
	"reference_escapes_domain":     "Reference の源が解決ディレクトリの外である",
	"reference_not_found":          "Reference の源ファイルが存在しない",
	"reference_encoding":           "Reference の源が UTF-8 ではない",
	"reference_span_invalid":       "Reference の範囲が源の外である",
	"relation_kind_invalid":        "順序の種別が契約にない",
	"relation_endpoints_invalid":   "順序の両端が使えない",
	"cross_domain_next":            "次発話は同一ドメインに限る",
	"object_not_found":             "対象が存在しない",
	"limit_invalid":                "検索上限が 1 以上 20 以下ではない",
	"domain_invalid":               "付け替え先が正規化できるディレクトリではない",
	"embed_provider_unset":         "埋め込みプロバイダまたはモデルが未設定である",
	"embed_unreachable":            "埋め込みプロバイダへ接続できない",
	"embed_rejected":               "埋め込みプロバイダがベクトルを返さなかった",
	"embed_dimension_mismatch":     "埋め込みの次元が索引と一致しない",
	"embed_cache_missing":          "検索対象の埋め込みキャッシュが欠けている",
	"embed_fixture_invalid":        "フィクスチャのベクトル定義が契約と違う",
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorDocument struct {
	Error errorBody `json:"error"`
}

func writeReject(stdout io.Writer, rej *rejection) {
	writeOK(stdout, errorDocument{Error: errorBody{Code: rej.code, Message: rej.message}})
}

func writeOK(stdout io.Writer, v any) {
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

type options struct {
	dir      string
	provider string
	model    string
	baseURL  string
	keyEnv   string
	fixture  string
	command  string
	seen     map[string]bool
}

func parseArgs(args []string) (options, *rejection) {
	opt := options{seen: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--") {
			name, val, inline := strings.Cut(arg, "=")
			if !inline {
				if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
					return options{}, reject("invalid_flag", 2)
				}
				i++
				val = args[i]
			}
			if val == "" {
				return options{}, reject("invalid_flag", 2)
			}
			if opt.seen[name] || opt.command != "" {
				return options{}, reject("invalid_flag", 2)
			}
			opt.seen[name] = true
			switch name {
			case "--dir":
				opt.dir = val
			case "--embed-provider":
				switch val {
				case "ollama", "openai", "fixture":
					opt.provider = val
				default:
					return options{}, reject("invalid_flag", 2)
				}
			case "--embed-model":
				opt.model = val
			case "--embed-base-url":
				opt.baseURL = val
			case "--embed-api-key-env":
				opt.keyEnv = val
			case "--embed-fixture":
				opt.fixture = val
			default:
				return options{}, reject("invalid_flag", 2)
			}
			continue
		}
		if opt.command != "" {
			return options{}, reject("invalid_command", 2)
		}
		opt.command = arg
	}
	if opt.command == "" {
		return options{}, reject("invalid_command", 2)
	}
	return opt, nil
}

func readObject(stdin io.Reader) (map[string]json.RawMessage, *rejection) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return nil, reject("invalid_json", 2)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value json.RawMessage
	if err := dec.Decode(&value); err != nil {
		return nil, reject("invalid_json", 2)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, reject("invalid_json", 2)
	}
	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, reject("invalid_json", 2)
	}
	objDec := json.NewDecoder(bytes.NewReader(trimmed))
	objDec.UseNumber()
	var fields map[string]json.RawMessage
	if err := objDec.Decode(&fields); err != nil {
		return nil, reject("invalid_json", 2)
	}
	return fields, nil
}
