package cli

import "strings"

// commandSpec is one entry in the human and machine command registry.
type commandSpec struct {
	name    string
	summary string
	usage   string
	help    string
	json    bool
	flags   []string
}

func commandRegistry() []commandSpec {
	return []commandSpec{
		{name: "init", summary: "解決ディレクトリのドメインを作る", usage: "agmemx [全体フラグ] init", json: true, help: initHelp},
		{name: "observe", summary: "観測を一つ作る", usage: "agmemx [全体フラグ] observe [フィールドフラグ]", json: true, flags: []string{"--text", "--source", "--start", "--end", "--interval-start", "--interval-end"}, help: observeHelp},
		{name: "belief add", summary: "信念を一つ作る", usage: "agmemx [全体フラグ] belief add [フィールドフラグ]", json: true, flags: []string{"--text", "--reason-kind", "--reason-id", "--reason-text", "--about", "--interval-start", "--interval-end"}, help: beliefHelp},
		{name: "relation add", summary: "同一ドメインの次発話を結ぶ", usage: "agmemx [全体フラグ] relation add [フィールドフラグ]", json: true, flags: []string{"--kind", "--from", "--to"}, help: relationHelp},
		{name: "search", summary: "自然文で部分木を検索する", usage: "agmemx [全体フラグ] search [フィールドフラグ]", json: true, flags: []string{"--query", "--limit"}, help: searchHelp},
		{name: "domain attach", summary: "所属の索引を付け替える", usage: "agmemx [全体フラグ] domain attach [フィールドフラグ]", json: true, flags: []string{"--id", "--domain"}, help: domainHelp},
		{name: "embed reindex", summary: "現在のプロバイダとモデルでキャッシュを作り直す", usage: "agmemx [全体フラグ] embed reindex", json: true, help: reindexHelp},
		{name: "config", summary: "埋め込みの既定を保存する", usage: "agmemx config [show|path|set KEY VALUE|unset KEY]", help: configHelp},
		{name: "help", summary: "目録、コマンド、concepts、usecases を表示する", usage: "agmemx help [TOPIC]", help: helpHelp},
		{name: "schema", summary: "機械向け JSON の形を表示する", usage: "agmemx schema", help: schemaHelp},
	}
}

func helpText(topic string) (string, bool) {
	topic = strings.TrimSpace(topic)
	switch topic {
	case "", "help":
		return catalogHelp(), true
	case "concepts":
		return conceptsHelp, true
	case "usecases":
		return usecasesHelp, true
	}
	for _, spec := range commandRegistry() {
		if spec.name == topic {
			return renderCommandHelp(spec), true
		}
	}
	alias := map[string]string{
		"believe":       "belief add",
		"relate":        "relation add",
		"domain-attach": "domain attach",
		"reindex":       "embed reindex",
		"belief":        "belief add",
		"relation":      "relation add",
		"domain":        "domain attach",
		"embed":         "embed reindex",
	}
	if name, ok := alias[topic]; ok {
		for _, spec := range commandRegistry() {
			if spec.name == name {
				return renderCommandHelp(spec), true
			}
		}
	}
	return "", false
}

func catalogHelp() string {
	var b strings.Builder
	b.WriteString("agmemx — 信念と観測の記憶\n\n")
	b.WriteString("Usage:\n  agmemx [全体フラグ] COMMAND [SUBCOMMAND] [フィールドフラグ]\n  agmemx help [TOPIC]\n\n")
	b.WriteString("Commands:\n")
	for _, spec := range commandRegistry() {
		b.WriteString("  " + spec.name + "  " + spec.summary + "\n")
	}
	b.WriteString("\n互換:\n")
	b.WriteString("  believe        belief add\n")
	b.WriteString("  relate         relation add\n")
	b.WriteString("  domain-attach  domain attach\n")
	b.WriteString("  reindex        embed reindex\n")
	b.WriteString("\nTopics:\n  agmemx help concepts\n  agmemx help usecases\n")
	b.WriteString("\n全体フラグはサブコマンドより前に置きます。--format が無く標準出力が TTY でなければ json です。\n")
	return b.String()
}

func renderCommandHelp(spec commandSpec) string {
	var b strings.Builder
	b.WriteString(spec.name + " — " + spec.summary + "\n\n")
	b.WriteString("Usage:\n  " + spec.usage + "\n")
	if len(spec.flags) != 0 {
		b.WriteString("\nフィールドフラグ:\n")
		for _, flag := range spec.flags {
			b.WriteString("  " + flag + "\n")
		}
	}
	if spec.json {
		b.WriteString("\nフィールドフラグが無いとき、標準入力の JSON オブジェクトを一つ読みます。フラグと JSON を同時には渡しません。\n")
	}
	b.WriteString("\n")
	b.WriteString(spec.help)
	if !strings.HasSuffix(spec.help, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

const initHelp = `解決ディレクトリのドメインを作ります。既にあるときも成功します。オブジェクトは作りません。
JSON は {} です。
`

const observeHelp = `観測を一つ作ります。理由は受け付けません。
フラグは --text、--source、--start、--end が必須です。期間は --interval-start と --interval-end を両方渡します。
JSON の鍵は text、reference、interval です。reason があると観測は拒否され、オブジェクトは残りません。
`

const beliefHelp = `信念を一つ作ります。互換コマンド believe も同じ操作です。
理由は --reason-kind belief|observation|text と、--reason-id または --reason-text です。--about は繰り返せます。
矛盾は、対象を --about に並べた信念として表します。
`

const relationHelp = `同一ドメインの次発話を結びます。互換コマンド relate も同じ操作です。
--kind next、--from、--to を渡します。別ドメインの両端は拒否します。
`

const searchHelp = `解決ディレクトリの部分木を、埋め込みの類似度で検索します。
検索語は引数です。標準入力は読みません。--query でも渡せます。--limit の既定は 8、最大は 20 です。
信念と観測を分けて返します。Reference は source、start、end だけで、源ファイルの本文は返しません。
`

const domainHelp = `所属の索引だけを付け替えます。互換コマンド domain-attach も同じ操作です。
--id と、絶対パスの --domain を渡します。content_sha256 は変わりません。
`

const reindexHelp = `部分木の記録を、現在のプロバイダとモデルでキャッシュし直します。互換コマンド reindex も同じ操作です。
JSON は {} です。失敗したときは新しいキャッシュを残しません。
`

const helpHelp = `トピックを省略すると目録を出します。concepts は語の意味、usecases は手順です。
コマンド名を渡すと、そのコマンドの --help と同じ本文です。ストアも埋め込みプロバイダも開きません。
`

const schemaHelp = `機械向け JSON のフィールド名を出します。ストアは開きません。
`

const configHelp = `埋め込みの既定を ${XDG_CONFIG_HOME:-$HOME/.config}/agmemx/config.json に保存します。
指定が無いときのプロバイダは ollama、そのモデルは nomic-embed-text です。
フラグは設定ファイルより優先します。設定ファイルはフラグが無いときより優先します。
鍵は embed-provider、embed-model、embed-base-url、embed-api-key-env です。
`

const conceptsHelp = `concepts — 語の意味

観測は、発話または文書のスパンを直接の理由に持つ記録です。信念を理由にはしません。
信念の理由は、無し、信念、観測、自然文のいずれかです。
矛盾は専用の状態ではなく、対象へリンクした信念です。
ドメインは解決ディレクトリの正規化絶対パスです。既定の検索はそのディレクトリと子孫です。
Reference は源と範囲を指し、本文は複製しません。
期間は渡した文字列を保存して返します。意味は決めません。
埋め込みプロバイダは ollama、openai、fixture のいずれか一つです。
`

const usecasesHelp = `usecases — 手順

ローカルの Ollama で記録する。プロバイダとモデルは既定で ollama と nomic-embed-text です:
  agmemx init
  agmemx observe --text "今朝はよく眠れた" --source notes/a.txt --start 0 --end 8
  agmemx search "昨夜の睡眠" --limit 2

モデルを変える:
  agmemx config set embed-model nomic-embed-text

フィクスチャで受入と同じ JSON を渡す:
  agmemx --format json --embed-provider fixture --embed-model fixture-model --embed-fixture vectors.json observe

ヘルプは記憶を読みません:
  agmemx help concepts
  agmemx observe --help
`
