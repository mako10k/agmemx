# CLI 表面

この文書は、記憶の意味を変えないまま、人が `agmemx` を手引き、補完、エラー案内から使える形に固定する。機械向けの入出力は [cli-contract.md](cli-contract.md) の JSON 契約を維持する。配置とドメインは [architecture.md](architecture.md) のままである。

参考にした形は次のとおり。

- perttool は `help` と `guide` をコマンド目録にし、診断がローカルな help topic を指す。`--format json` が機械向けである。
- sealgraph は `help`、`<command> --help`、`hint:`、`help:` を同じ topic に揃える。案内は修復を実行しない。Bash 補完は薄いラッパーと、バイナリの `__completion --bash` である。
- secdat は `help concepts` と `help usecases`、man、`__completion --bash`、近いコマンド名の候補表示を持つ。

## 二つの出口

`--format json` または `--format text` を指定したときはそれに従う。指定がなく、標準出力が TTY でないときは json である。指定がなく、標準出力が TTY のときは text である。

json の成功と拒否は、標準出力に JSON オブジェクトを一つ書く。終了コード 0 と 1 の標準エラーは空である。拒否オブジェクトは `code` と `message` の二鍵だけであり、現行契約の文面を変えない。

text の成功は短い人向け表示である。text の拒否は標準エラーに次の形で書き、標準出力には結果を書かない。

```text
error: 観測の理由は受け付けない
hint: 観測から reason を外す。拒否のときオブジェクトは残らない。
help: agmemx help observe
```

`hint` は次に読むコマンドを示すだけで、登録、削除、再索引、プロバイダ呼び出しは行わない。未知のコマンドは実行せず、近いコマンド名を一つ `hint` に出す。`__completion` は候補に出さない。

## サブコマンド

全体フラグはサブコマンドより前に置く。

```text
agmemx [--dir PATH] [--format json|text] [--embed-provider NAME] [--embed-model NAME] [--embed-base-url URL] [--embed-api-key-env NAME] [--embed-fixture PATH] COMMAND [SUBCOMMAND] [フラグ]
```

| コマンド | 役割 |
| --- | --- |
| `init` | 解決ディレクトリのドメインを作る |
| `observe` | 観測を一つ作る |
| `belief add` | 信念を一つ作る |
| `relation add` | 同一ドメインの次発話を結ぶ |
| `search` | 自然文で部分木を検索する |
| `domain attach` | 所属の索引を付け替える |
| `embed reindex` | 現在のプロバイダとモデルでキャッシュを作り直す |
| `help [TOPIC]` | 目録、コマンド、`concepts`、`usecases` を表示する |
| `schema` | 機械向け JSON の形を表示する |
| `__completion --bash` | Bash 補完プロトコル。目録には出さない |

`observe --help` と `agmemx help observe` は同じ本文である。`belief --help` は `belief add` を示す。ヘルプはストアを開かず、埋め込みプロバイダを呼ばない。

フィールドはフラグで渡せる。フィールドフラグが無く、標準入力がパイプのときは、現行どおり JSON オブジェクトを一つ読む。フラグと JSON を同時に渡したときは `invalid_flag` で、どちらも実行しない。

次の綴りは互換として残す。ヘルプでは新しい綴りを先に出す。

| 互換 | 新しい綴り |
| --- | --- |
| `relate` | `relation add` |
| `domain-attach` | `domain attach` |
| `reindex` | `embed reindex` |

## 手引きとインストール

正本はバイナリのヘルプである。man は同じレジストリから作り、`docs/agmemx.1` に置く。コマンド一覧、全体フラグ、終了コード、`help` への参照を含む。

`make install PREFIX=$HOME/.local` は次を置く。

```text
$PREFIX/bin/agmemx
$PREFIX/share/bash-completion/completions/agmemx
$PREFIX/share/man/man1/agmemx.1
```

`make uninstall` はその三つのファイルを消す。サービス、フック、設定ファイルは置かない。

## Bash 補完

`completions/agmemx.bash` はシェル側の薄いラッパーである。候補の選択は `agmemx __completion --bash` が行う。出力の先頭行は `__agmemx_completion_mode=plain|file|dir|none` で、続きが候補である。

補完はコマンド、サブコマンド、フラグ、プロバイダ名を出す。`--dir` はディレクトリ、`--embed-fixture` と Reference の源はファイルである。記憶オブジェクト、鍵、埋め込みプロバイダは読まない。

## この計画で変えないもの

信念と観測の意味、Reference、理由の制約、ドメイン木、XDG、埋め込みのリクエスト形、拒否コードの文面は変えない。JSON モードの受入セッションは、`--format json` を明示しても同じ結果になる。
