package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// modeWriter is text mode. Rejections go to stderr. Success JSON becomes one
// short line. Help text and schema JSON pass through dst unchanged.
type modeWriter struct {
	dst     io.Writer
	stderr  io.Writer
	command string
	near    string
}

func (w *modeWriter) Write(p []byte) (int, error) {
	return w.dst.Write(p)
}

func selectFormat(explicit string, tty bool) string {
	switch explicit {
	case "json", "text":
		return explicit
	}
	if tty {
		return "text"
	}
	return "json"
}

// guide is the hint and help topic for one rejection code.
// Hints only say what to read. They do not run a command, write the store, or call a provider.
type guide struct {
	hint  string
	topic string
}

// errorGuides is the single table of hints and help topics.
// invalid_command's hint is a fmt template; its topic is the closest command.
// When the invocation already names an operational command, that command replaces topic.
var errorGuides = map[string]guide{
	"invalid_json":                 {hint: "標準入力は JSON オブジェクトを一つにする。", topic: "init"},
	"invalid_command":              {hint: "近いコマンドは %s である。実行はしない。", topic: "help"},
	"invalid_flag":                 {hint: "全体フラグはサブコマンドより前に置く。未知のフラグは外し、フラグと JSON は同時に渡さない。", topic: "init"},
	"unknown_field":                {hint: "契約にないフィールドを外す。", topic: "init"},
	"missing_field":                {hint: "必須フィールドを足す。", topic: "observe"},
	"invalid_type":                 {hint: "フィールドの型を契約に合わせる。", topic: "observe"},
	"xdg_relative":                 {hint: "XDG_DATA_HOME、XDG_CACHE_HOME、XDG_STATE_HOME は絶対パスにする。", topic: "init"},
	"dir_not_found":                {hint: "--dir には存在するディレクトリを渡す。", topic: "init"},
	"dir_not_directory":            {hint: "--dir にはディレクトリを渡す。", topic: "init"},
	"dir_is_root":                  {hint: "ファイルシステムのルートはドメインにしない。", topic: "init"},
	"text_empty":                   {hint: "本文を空にしない。", topic: "observe"},
	"text_too_long":                {hint: "本文は8192文字以内にする。", topic: "observe"},
	"interval_invalid":             {hint: "期間は RFC3339 で、終了を開始より前にしない。", topic: "observe"},
	"observation_reason_forbidden": {hint: "観測から reason を外す。拒否のときオブジェクトは残らない。", topic: "observe"},
	"reason_kind_invalid":          {hint: "理由種別は belief、observation、text のいずれかにする。", topic: "belief add"},
	"reason_not_found":             {hint: "理由の対象は同じ部分木にある信念か観測にする。", topic: "belief add"},
	"reference_escapes_domain":     {hint: "Reference の源は解決ディレクトリの中の相対パスにする。", topic: "observe"},
	"reference_not_found":          {hint: "Reference の源ファイルを解決ディレクトリに置く。", topic: "observe"},
	"reference_encoding":           {hint: "Reference の源を UTF-8 にする。", topic: "observe"},
	"reference_span_invalid":       {hint: "Reference の start と end を源の文字範囲の内側にする。", topic: "observe"},
	"relation_kind_invalid":        {hint: "順序の種別は next にする。", topic: "relation add"},
	"relation_endpoints_invalid":   {hint: "両端は異なる 32 桁の 16 進 ID にする。", topic: "relation add"},
	"cross_domain_next":            {hint: "次発話の両端を同じドメインにする。", topic: "relation add"},
	"object_not_found":             {hint: "対象は解決ディレクトリの部分木にある ID にする。", topic: "domain attach"},
	"limit_invalid":                {hint: "検索上限は 1 以上 20 以下にする。", topic: "search"},
	"domain_invalid":               {hint: "付け替え先はルート以外の、存在する絶対パスのディレクトリにする。", topic: "domain attach"},
	"embed_provider_unset":         {hint: "既定は ollama と nomic-embed-text である。fixture には --embed-fixture と --embed-model も要る。", topic: "config"},
	"embed_unreachable":            {hint: "埋め込みプロバイダへは接続しない。到達できる設定を案内で確認する。", topic: "search"},
	"embed_rejected":               {hint: "プロバイダはここでは呼ばない。ベクトルを含む応答かを案内で確認する。", topic: "search"},
	"embed_dimension_mismatch":     {hint: "次元の違う索引はここでは作り直さない。embed reindex の案内を読む。", topic: "embed reindex"},
	"embed_cache_missing":          {hint: "欠けたキャッシュはここでは埋め直さない。検索の案内を読む。", topic: "search"},
	"embed_fixture_invalid":        {hint: "フィクスチャは model、dimension、vectors の契約どおりにする。", topic: "embed reindex"},
}

var operationalTopics = map[string]struct{}{
	"init":          {},
	"observe":       {},
	"belief add":    {},
	"relation add":  {},
	"search":        {},
	"domain attach": {},
	"embed reindex": {},
	"config":        {},
}

func textReject(code, message, command, near string) string {
	hint, topic := guideFor(code, command, near)
	return "error: " + message + "\n" +
		"hint: " + hint + "\n" +
		"help: agmemx help " + topic + "\n"
}

func guideFor(code, command, near string) (string, string) {
	entry, ok := errorGuides[code]
	if !ok {
		return "案内を読む。", "init"
	}
	if code == "invalid_command" {
		name := closestCommand(near)
		return fmt.Sprintf(entry.hint, name), name
	}
	if topic, ok := operationalTopic(command); ok {
		return entry.hint, topic
	}
	return entry.hint, entry.topic
}

func operationalTopic(command string) (string, bool) {
	if command == "" {
		return "", false
	}
	topic := helpTopicFor(command)
	if _, ok := operationalTopics[topic]; ok {
		return topic, true
	}
	return "", false
}

type commandCandidate struct {
	key  string
	name string
	rank int
}

func commandCandidates() []commandCandidate {
	var out []commandCandidate
	rank := map[string]int{}
	for i, spec := range commandRegistry() {
		if spec.name == "__completion" {
			continue
		}
		rank[spec.name] = i
		out = append(out, commandCandidate{key: spec.name, name: spec.name, rank: i})
	}
	aliases := []struct{ key, name string }{
		{"believe", "belief add"},
		{"relate", "relation add"},
		{"domain-attach", "domain attach"},
		{"reindex", "embed reindex"},
		{"belief", "belief add"},
		{"relation", "relation add"},
		{"domain", "domain attach"},
		{"embed", "embed reindex"},
	}
	for _, alias := range aliases {
		out = append(out, commandCandidate{key: alias.key, name: alias.name, rank: rank[alias.name]})
	}
	return out
}

func closestCommand(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return "help"
	}
	bestName := "init"
	bestDist := int(^uint(0) >> 1)
	bestRank := int(^uint(0) >> 1)
	for _, candidate := range commandCandidates() {
		if candidate.name == "__completion" || candidate.key == "__completion" {
			continue
		}
		dist := levenshtein(input, candidate.key)
		if dist < bestDist || (dist == bestDist && candidate.rank < bestRank) {
			bestDist = dist
			bestName = candidate.name
			bestRank = candidate.rank
		}
	}
	return bestName
}

func levenshtein(left, right string) int {
	a := []rune(left)
	b := []rune(right)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

func formatTextSuccess(v any) (string, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", false
	}
	return formatMarshaledSuccess(raw)
}

func formatMarshaledSuccess(raw []byte) (string, bool) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		return "", false
	}
	if _, ok := doc["commands"]; ok {
		return "", false
	}
	if _, ok := doc["beliefs"]; ok {
		text, err := formatSearch(raw)
		if err != nil {
			return "", false
		}
		return text, true
	}
	if rawCount, ok := doc["count"]; ok {
		var count int
		if err := json.Unmarshal(rawCount, &count); err != nil {
			return "", false
		}
		return fmt.Sprintf("reindexed %d\n", count), true
	}
	if _, ok := doc["from"]; ok {
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", false
		}
		return "next " + body.From + " " + body.To + "\n", true
	}
	if _, ok := doc["content_sha256"]; ok {
		var body struct {
			ID     string `json:"id"`
			Kind   string `json:"kind"`
			Domain string `json:"domain"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", false
		}
		if _, hasKind := doc["kind"]; hasKind {
			return body.Kind + " " + body.ID + "\n", true
		}
		return "attached " + body.ID + " " + body.Domain + "\n", true
	}
	if rawDomain, ok := doc["domain"]; ok && len(doc) == 1 {
		var domain string
		if err := json.Unmarshal(rawDomain, &domain); err != nil {
			return "", false
		}
		return "domain " + domain + "\n", true
	}
	return "", false
}

type textHit struct {
	score    float64
	scoreRaw string
	kind     string
	id       string
	text     string
}

func formatSearch(raw []byte) (string, error) {
	var doc struct {
		Beliefs []struct {
			Score json.RawMessage `json:"score"`
			Kind  string          `json:"kind"`
			ID    string          `json:"id"`
			Text  string          `json:"text"`
		} `json:"beliefs"`
		Observations []struct {
			Score json.RawMessage `json:"score"`
			Kind  string          `json:"kind"`
			ID    string          `json:"id"`
			Text  string          `json:"text"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", err
	}
	var hits []textHit
	add := func(score json.RawMessage, kind, id, text string) error {
		var value float64
		if err := json.Unmarshal(score, &value); err != nil {
			return err
		}
		hits = append(hits, textHit{
			score:    value,
			scoreRaw: strings.TrimSpace(string(score)),
			kind:     kind,
			id:       id,
			text:     oneLine(text),
		})
		return nil
	}
	for _, item := range doc.Beliefs {
		if err := add(item.Score, item.Kind, item.ID, item.Text); err != nil {
			return "", err
		}
	}
	for _, item := range doc.Observations {
		if err := add(item.Score, item.Kind, item.ID, item.Text); err != nil {
			return "", err
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].id < hits[j].id
	})
	var b strings.Builder
	for _, hit := range hits {
		fmt.Fprintf(&b, "%s %s %s %s\n", hit.scoreRaw, hit.kind, hit.id, hit.text)
	}
	return b.String(), nil
}

func oneLine(text string) string {
	text = strings.ReplaceAll(text, "\r\n", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	return text
}
