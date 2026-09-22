package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectionGuides(t *testing.T) {
	if len(errorGuides) != len(messages) {
		t.Fatalf("guides %d messages %d", len(errorGuides), len(messages))
	}
	for code, message := range messages {
		entry, ok := errorGuides[code]
		if !ok {
			t.Fatalf("missing guide %s", code)
		}
		if entry.hint == "" || strings.Contains(entry.hint, "\n") {
			t.Fatalf("hint %s %q", code, entry.hint)
		}
		if strings.HasPrefix(strings.TrimSpace(entry.hint), "agmemx ") {
			t.Fatalf("hint executes a command: %s", entry.hint)
		}
		if code == "invalid_command" {
			if !strings.Contains(entry.hint, "%s") {
				t.Fatalf("invalid_command hint %q", entry.hint)
			}
			if entry.topic != "help" {
				t.Fatalf("empty-command topic %s", entry.topic)
			}
			continue
		}
		if _, ok := operationalTopics[entry.topic]; !ok {
			t.Fatalf("topic %s for %s", entry.topic, code)
		}
		if _, ok := helpText(entry.topic); !ok {
			t.Fatalf("help %s", entry.topic)
		}
		got := textReject(code, message, "", "")
		want := "error: " + message + "\nhint: " + entry.hint + "\nhelp: agmemx help " + entry.topic + "\n"
		if got != want {
			t.Fatalf("%s\n%q\n%q", code, got, want)
		}
	}
	got := textReject("invalid_json", messages["invalid_json"], "observe", "")
	if !strings.Contains(got, "help: agmemx help observe\n") {
		t.Fatalf("command topic %q", got)
	}
	got = textReject("missing_field", messages["missing_field"], "believe", "")
	if !strings.Contains(got, "help: agmemx help belief add\n") {
		t.Fatalf("belief topic %q", got)
	}
	got = textReject("observation_reason_forbidden", messages["observation_reason_forbidden"], "observe", "")
	want := "error: 観測の理由は受け付けない\nhint: 観測から reason を外す。拒否のときオブジェクトは残らない。\nhelp: agmemx help observe\n"
	if got != want {
		t.Fatalf("%q", got)
	}
}

func TestClosestRegistryCommand(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "help"},
		{"observ", "observe"},
		{"serch", "search"},
		{"reindex", "embed reindex"},
		{"domain-attach", "domain attach"},
		{"relat", "relation add"},
		{"believ", "belief add"},
	}
	names := map[string]bool{}
	for _, spec := range commandRegistry() {
		names[spec.name] = true
		if spec.name == "__completion" {
			t.Fatal("registry lists __completion")
		}
	}
	for _, tc := range cases {
		got := closestCommand(tc.input)
		if got != tc.want {
			t.Fatalf("%q -> %q want %q", tc.input, got, tc.want)
		}
		if !names[got] {
			t.Fatalf("%q suggested %q", tc.input, got)
		}
	}
	for _, input := range []string{"nope", "__completion", "__completion --bash"} {
		got := closestCommand(input)
		if got == "__completion" || !names[got] {
			t.Fatalf("%q -> %q", input, got)
		}
		hint, topic := guideFor("invalid_command", "", input)
		if topic != got || strings.Contains(hint, "__completion") {
			t.Fatalf("hint %q topic %q", hint, topic)
		}
		assertOneCommand(t, hint, got)
	}
}

func TestSelectFormat(t *testing.T) {
	if selectFormat("", false) != "json" || selectFormat("", true) != "text" {
		t.Fatal("default format")
	}
	if selectFormat("json", true) != "json" || selectFormat("text", false) != "text" {
		t.Fatal("explicit format")
	}
	var buf bytes.Buffer
	if stdoutIsTerminal(&buf) {
		t.Fatal("buffer is not a terminal")
	}
}

func TestFormatTextSuccess(t *testing.T) {
	got, ok := formatTextSuccess(map[string]any{"domain": "/tmp/memory"})
	if !ok || got != "domain /tmp/memory\n" {
		t.Fatalf("init %q %v", got, ok)
	}
	got, ok = formatTextSuccess(map[string]any{
		"id": "abc", "kind": "belief", "domain": "/tmp/memory", "content_sha256": "ff",
	})
	if !ok || got != "belief abc\n" {
		t.Fatalf("belief %q", got)
	}
	got, ok = formatTextSuccess(map[string]any{
		"kind": "next", "from": "aaa", "to": "bbb", "domain": "/tmp/memory",
	})
	if !ok || got != "next aaa bbb\n" {
		t.Fatalf("relate %q", got)
	}
	got, ok = formatTextSuccess(map[string]any{
		"id": "abc", "domain": "/other", "content_sha256": "ff",
	})
	if !ok || got != "attached abc /other\n" {
		t.Fatalf("attach %q", got)
	}
	got, ok = formatTextSuccess(map[string]any{"provider": "fixture", "model": "m", "count": 4})
	if !ok || got != "reindexed 4\n" {
		t.Fatalf("reindex %q", got)
	}
	_, ok = formatTextSuccess(map[string]any{"commands": []any{}})
	if ok {
		t.Fatal("schema should stay JSON")
	}
	raw := []byte(`{"beliefs":[{"id":"bbb","kind":"belief","score":0.5,"text":"hold tight"}],"observations":[{"id":"ccc","kind":"observation","score":1,"text":"see\nthis"},{"id":"aaa","kind":"observation","score":1,"text":"see this"}]}`)
	lines, err := formatSearch(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := "1 observation aaa see this\n1 observation ccc see this\n0.5 belief bbb hold tight\n"
	if lines != want {
		t.Fatalf("search\n%q\n%q", lines, want)
	}
}

func TestTextModeRejections(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	env := textEnv(data)

	stdout, stderr, code := runText(t, []string{"--format", "text", "observ"}, "{}", env, dir)
	if code != 2 || stdout != "" {
		t.Fatalf("observ code %d stdout %q stderr %q", code, stdout, stderr)
	}
	want := "error: 未知のコマンドである\nhint: 近いコマンドは observe である。実行はしない。\nhelp: agmemx help observe\n"
	if stderr != want {
		t.Fatalf("observ stderr %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(data, "agmemx")); !os.IsNotExist(err) {
		t.Fatal("unknown command wrote the store")
	}

	stdout, stderr, code = runText(t, []string{"--format", "text", "__completion"}, "", env, dir)
	if code != 2 || stdout != "" || strings.Contains(stderr, "__completion") {
		t.Fatalf("__completion code %d stdout %q stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "hint: 近いコマンドは ") || !strings.Contains(stderr, "help: agmemx help ") {
		t.Fatalf("__completion stderr %q", stderr)
	}

	stdout, stderr, code = runText(t, []string{"--format", "text"}, "", env, dir)
	if code != 2 || stdout != "" {
		t.Fatalf("empty code %d stdout %q", code, stdout)
	}
	if stderr != "error: 未知のコマンドである\nhint: 近いコマンドは help である。実行はしない。\nhelp: agmemx help help\n" {
		t.Fatalf("empty stderr %q", stderr)
	}

	stdout, stderr, code = runText(t, []string{"--format", "json", "nope"}, "{}", env, dir)
	if code != 2 || stderr != "" {
		t.Fatalf("json nope %d %q %q", code, stdout, stderr)
	}
	assertJSONError(t, stdout, "invalid_command")

	stdout, stderr, code = runText(t, []string{"nope"}, "{}", env, dir)
	if code != 2 || stderr != "" {
		t.Fatalf("default nope %d %q %q", code, stdout, stderr)
	}
	assertJSONError(t, stdout, "invalid_command")

	stdout, stderr, code = runText(t, []string{"--format", "text", "--dir", dir, "observe"}, `{"reason":{"kind":"text","text":"because"}}`, env, dir)
	if code != 1 || stdout != "" {
		t.Fatalf("reason code %d stdout %q stderr %q", code, stdout, stderr)
	}
	if stderr != "error: 観測の理由は受け付けない\nhint: 観測から reason を外す。拒否のときオブジェクトは残らない。\nhelp: agmemx help observe\n" {
		t.Fatalf("reason stderr %q", stderr)
	}
	if _, err := os.Stat(filepath.Join(data, "agmemx")); !os.IsNotExist(err) {
		t.Fatal("reason rejection wrote the store")
	}

	stdout, stderr, code = runText(t, []string{"--format", "json", "--dir", dir, "observe"}, `{"reason":{"kind":"text","text":"because"}}`, env, dir)
	if code != 1 || stderr != "" {
		t.Fatalf("json reason %d %q %q", code, stdout, stderr)
	}
	assertJSONError(t, stdout, "observation_reason_forbidden")

	stdout, stderr, code = runText(t, []string{"--format", "text", "--dir", dir, "observe"}, "{", env, dir)
	if code != 2 || stdout != "" || !strings.Contains(stderr, "help: agmemx help observe\n") {
		t.Fatalf("bad json %d %q %q", code, stdout, stderr)
	}
	if !strings.HasPrefix(stderr, "error: 入力は1つの JSON オブジェクトである\n") {
		t.Fatalf("bad json stderr %q", stderr)
	}

	stdout, stderr, code = runText(t, []string{"--format", "text", "--dir", "/", "init"}, "{}", env, dir)
	if code != 1 || stdout != "" {
		t.Fatalf("root %d %q %q", code, stdout, stderr)
	}
	if stderr != "error: 解決ディレクトリがファイルシステムのルートである\nhint: ファイルシステムのルートはドメインにしない。\nhelp: agmemx help init\n" {
		t.Fatalf("root stderr %q", stderr)
	}
}

func TestTextModeSuccess(t *testing.T) {
	data := t.TempDir()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("FILEBODY-NOT-MEMORY"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	writeTextFixture(t, fixture, map[string][]float64{
		"seen one":    {1, 0},
		"seen two":    {1, 0},
		"held belief": {0, 1},
	})
	env := textEnv(data)
	base := []string{"--dir", dir, "--embed-provider", "fixture", "--embed-model", "fixture-model", "--embed-fixture", fixture}

	jsonOut, jsonErr, jsonCode := runText(t, append(append([]string{}, base...), "init"), "{}", env, dir)
	if jsonCode != 0 || jsonErr != "" {
		t.Fatalf("json init %d %s %s", jsonCode, jsonOut, jsonErr)
	}
	var initBody struct {
		Domain string `json:"domain"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &initBody); err != nil {
		t.Fatal(err)
	}
	textOut, textErr, textCode := runText(t, append(append([]string{"--format", "text"}, base...), "init"), "{}", env, dir)
	if textCode != 0 || textErr != "" || textOut != "domain "+initBody.Domain+"\n" {
		t.Fatalf("text init %d %q %q", textCode, textOut, textErr)
	}

	observeBody := `{"text":"seen one","reference":{"source":"a.txt","start":0,"end":4}}`
	jsonOut, jsonErr, jsonCode = runText(t, append(append([]string{}, base...), "observe"), observeBody, env, dir)
	if jsonCode != 0 || jsonErr != "" || !strings.Contains(jsonOut, `"kind":"observation"`) {
		t.Fatalf("json observe %d %s %s", jsonCode, jsonOut, jsonErr)
	}
	var observed struct {
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &observed); err != nil {
		t.Fatal(err)
	}

	textOut, textErr, textCode = runText(t, append(append([]string{"--format", "text"}, base...), "observe"), `{"text":"seen two","reference":{"source":"a.txt","start":0,"end":4}}`, env, dir)
	if textCode != 0 || textErr != "" {
		t.Fatalf("text observe %d %q %q", textCode, textOut, textErr)
	}
	secondID, ok := cutID(textOut, "observation")
	if !ok {
		t.Fatalf("observation line %q", textOut)
	}

	textOut, textErr, textCode = runText(t, append(append([]string{"--format", "text"}, base...), "belief", "add"), `{"text":"held belief"}`, env, dir)
	if textCode != 0 || textErr != "" {
		t.Fatalf("text belief %d %q %q", textCode, textOut, textErr)
	}
	beliefID, ok := cutID(textOut, "belief")
	if !ok {
		t.Fatalf("belief line %q", textOut)
	}

	textOut, textErr, textCode = runText(t, append(append([]string{"--format", "text"}, base...), "relation", "add"), `{"kind":"next","from":"`+secondID+`","to":"`+beliefID+`"}`, env, dir)
	if textCode != 0 || textErr != "" || textOut != "next "+secondID+" "+beliefID+"\n" {
		t.Fatalf("text relate %d %q %q", textCode, textOut, textErr)
	}

	searchArgs := append(append([]string{}, base...), "search", "--query", "seen one")
	jsonOut, jsonErr, jsonCode = runText(t, searchArgs, ``, env, dir)
	if jsonCode != 0 || jsonErr != "" {
		t.Fatalf("json search %d %s %s", jsonCode, jsonOut, jsonErr)
	}
	wantLines, err := formatSearch([]byte(jsonOut))
	if err != nil {
		t.Fatal(err)
	}
	textOut, textErr, textCode = runText(t, append(append([]string{"--format", "text"}, base...), "search", "--query", "seen one"), ``, env, dir)
	if textCode != 0 || textErr != "" || textOut != wantLines {
		t.Fatalf("text search %d %q %q want %q", textCode, textOut, textErr, wantLines)
	}
	if !strings.Contains(textOut, observed.Kind+" "+observed.ID+" seen one\n") {
		t.Fatalf("missing observation line %q", textOut)
	}
	if strings.Contains(textOut, "{") || strings.Contains(textOut, "FILEBODY-NOT-MEMORY") {
		t.Fatalf("search leaked %q", textOut)
	}

	textOut, textErr, textCode = runText(t, append(append([]string{"--format", "text"}, base...), "embed", "reindex"), "{}", env, dir)
	if textCode != 0 || textErr != "" || textOut != "reindexed 3\n" {
		t.Fatalf("text reindex %d %q %q", textCode, textOut, textErr)
	}

	target := t.TempDir()
	textOut, textErr, textCode = runText(t, append(append([]string{"--format", "text"}, base...), "domain", "attach"), `{"id":"`+beliefID+`","domain":"`+target+`"}`, env, dir)
	if textCode != 0 || textErr != "" {
		t.Fatalf("text attach %d %q %q", textCode, textOut, textErr)
	}
	if !strings.HasPrefix(textOut, "attached "+beliefID+" ") || strings.Contains(textOut, "{") {
		t.Fatalf("attach line %q", textOut)
	}
	attachedDomain := strings.TrimPrefix(strings.TrimSuffix(textOut, "\n"), "attached "+beliefID+" ")
	if !filepath.IsAbs(attachedDomain) || attachedDomain == "/" {
		t.Fatalf("domain %q", attachedDomain)
	}

	schema, schemaErr, schemaCode := runText(t, []string{"schema"}, "", env, dir)
	textSchema, textSchemaErr, textSchemaCode := runText(t, []string{"--format", "text", "schema"}, "", env, dir)
	jsonSchema, jsonSchemaErr, jsonSchemaCode := runText(t, []string{"--format", "json", "schema"}, "", env, dir)
	if schemaCode != 0 || textSchemaCode != 0 || jsonSchemaCode != 0 || schemaErr != "" || textSchemaErr != "" || jsonSchemaErr != "" {
		t.Fatalf("schema %d %d %d", schemaCode, textSchemaCode, jsonSchemaCode)
	}
	if schema != textSchema || schema != jsonSchema {
		t.Fatal("schema changed in text mode")
	}
	if !strings.Contains(schema, `"belief add"`) {
		t.Fatalf("schema %s", schema)
	}

	helpOut, helpErr, helpCode := runText(t, []string{"help"}, "", env, dir)
	textHelp, textHelpErr, textHelpCode := runText(t, []string{"--format", "text", "help"}, "", env, dir)
	jsonHelp, jsonHelpErr, jsonHelpCode := runText(t, []string{"--format", "json", "help"}, "", env, dir)
	if helpCode != 0 || textHelpCode != 0 || jsonHelpCode != 0 || helpErr != "" || textHelpErr != "" || jsonHelpErr != "" {
		t.Fatalf("help codes %d %d %d", helpCode, textHelpCode, jsonHelpCode)
	}
	if helpOut != textHelp || helpOut != jsonHelp || !strings.Contains(helpOut, "belief add") {
		t.Fatal("help changed in text mode")
	}
}

func assertOneCommand(t *testing.T, hint, name string) {
	t.Helper()
	if !strings.Contains(hint, name) {
		t.Fatalf("hint %q missing %s", hint, name)
	}
	for _, spec := range commandRegistry() {
		if spec.name == name {
			continue
		}
		if strings.Contains(hint, spec.name) {
			t.Fatalf("hint %q also names %s", hint, spec.name)
		}
	}
}

func assertJSONError(t *testing.T, stdout, code string) {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 {
		t.Fatalf("keys %#v", raw)
	}
	body, ok := raw["error"].(map[string]any)
	if !ok || len(body) != 2 || body["code"] != code || body["message"] != messages[code] {
		t.Fatalf("error %#v", raw["error"])
	}
}

func cutID(line, kind string) (string, bool) {
	line = strings.TrimSuffix(line, "\n")
	prefix := kind + " "
	if !strings.HasPrefix(line, prefix) || strings.Contains(line, "\n") {
		return "", false
	}
	id := strings.TrimPrefix(line, prefix)
	if len(id) != 32 {
		return "", false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		default:
			return "", false
		}
	}
	return id, true
}

func runText(t *testing.T, args []string, in string, env []string, cwd string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, strings.NewReader(in), &stdout, &stderr, env, cwd)
	return stdout.String(), stderr.String(), code
}

func textEnv(data string) []string {
	return []string{
		"HOME=" + data,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + data,
		"XDG_STATE_HOME=" + data,
	}
}

func writeTextFixture(t *testing.T, path string, vectors map[string][]float64) {
	t.Helper()
	body, err := json.Marshal(map[string]any{"model": "fixture-model", "dimension": 2, "vectors": vectors})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
