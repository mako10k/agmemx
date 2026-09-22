# CLI 契約

この文書は、受入セッションが使うコマンド、JSON、拒否コードを固定する。根拠は [architecture.md](architecture.md) と [requirements.md](requirements.md) である。実装は、ここに無いフィールドや修復手順を呼び出し側へ返さない。

期間は、呼び出し側が渡した文字列を保存し、同じ文字列で返す。期間が指す意味は、この契約では決めない。

## プロセス

実行ファイル名は `agmemx` である。

```text
agmemx [全体フラグ] COMMAND
```

標準入力は、そのコマンドの JSON オブジェクトをちょうど一つ置く。成功も拒否も、JSON オブジェクトを標準出力へ一つだけ書く。終了コード 0 と 1 のとき、標準エラーは空である。

終了コードは次のとおり。

| コード | 条件 |
| --- | --- |
| 0 | 成功 |
| 1 | 下の表で終了コード 1 の拒否 |
| 2 | 下の表で終了コード 2 の拒否 |

全体フラグは次だけを受け付ける。不明なフラグは `invalid_flag` である。

| フラグ | 意味 |
| --- | --- |
| `--dir PATH` | 解決ディレクトリ。無いときはプロセス起動時のカレントディレクトリ |
| `--embed-provider NAME` | `ollama`、`openai`、`fixture` のいずれか |
| `--embed-model NAME` | モデル名。三つのプロバイダすべてで必須 |
| `--embed-base-url URL` | `ollama` の既定は `http://127.0.0.1:11434`。`openai` の既定は `https://api.openai.com/v1` |
| `--embed-api-key-env NAME` | 鍵を読む環境変数名。`openai` の既定は `OPENAI_API_KEY`。`ollama` と `fixture` では使わない |
| `--embed-fixture PATH` | `fixture` のベクトル定義ファイル。`fixture` のとき必須 |

`openai` は OpenAI の `POST /v1/embeddings` と同じ形を送るプロバイダである。既定以外の host は `--embed-base-url` で指定する。xAI を使うときは base URL を `https://api.x.ai/v1`、鍵の環境変数を `XAI_API_KEY` にする。そのモデルが埋め込みを返すかは、指定したモデルの応答で決まる。

相対パスの `--dir` は、起動時のカレントディレクトリから解決して正規化する。正規化の結果がファイルシステムのルートなら `dir_is_root`、存在しなければ `dir_not_found`、ディレクトリでなければ `dir_not_directory` である。

`XDG_DATA_HOME`、`XDG_CACHE_HOME`、`XDG_STATE_HOME` は、未設定または空なら各仕様の既定値を使う。値が相対パスなら、コマンド本文を読む前に `xdg_relative` で拒否する。

登録と再索引は、埋め込みが成功するまでオブジェクトも新しいキャッシュも確定しない。拒否のあと、その呼び出しで増えたオブジェクトと、その呼び出しが作りかけたキャッシュ鍵は残らない。

## 共通の形

識別子は、実装が割り当てる小文字十六進 32 文字である。呼び出し側は識別子を送らない。

`content_sha256` は、ドメインと識別子を除いた内容の SHA-256 である。鍵を昇順にした、空白のない UTF-8 JSON をハッシュする。観測の内容鍵は `interval`、`reference`、`text` である。信念の内容鍵は `about`、`interval`、`reason`、`text` である。`about` は識別子の昇順、重複なしである。

期間を置くときは、オブジェクト `{"start":"...","end":"..."}` だけを認める。両方とも RFC3339 の時刻で、オフセットを含む。`end` は `start` 以上である。条件を外れるときは `interval_invalid` である。省略した期間は JSON の `null` で返す。

本文 `text` は空でなく、Unicode スカラー 8192 個以下である。空は `text_empty`、超過は `text_too_long` である。

未知のフィールドは `unknown_field`、欠落は `missing_field`、型の不一致は `invalid_type` である。数値は JSON の整数だけを、位置と上限に使う。

拒否オブジェクトは次の二鍵だけである。

```json
{"error":{"code":"text_empty","message":"本文が空である"}}
```

`message` は下の表の文面と一致し、識別子やパスを埋め込まない。

## コマンド

### init

本文は `{}` である。解決ディレクトリのドメインを作る。既にあるときも成功する。オブジェクトは作らない。

```json
{"domain":"/canonical/path"}
```

### observe

観測を一つ作る。埋め込む文字列は `text` だけである。

```json
{
  "text": "発話の記録",
  "reference": {"source": "notes/a.txt", "start": 0, "end": 4},
  "interval": {"start": "2026-01-01T00:00:00Z", "end": "2026-01-02T00:00:00Z"}
}
```

`reference.source` は、解決ディレクトリからの相対パスである。正規化の結果がそのディレクトリの外へ出るときは `reference_escapes_domain` である。ファイルが無いときは `reference_not_found`、UTF-8 でないときは `reference_encoding` である。`start` と `end` は Unicode スカラーの半開区間で、ファイル長の外なら `reference_span_invalid` である。

`interval` は省略できる。鍵 `reason` があるときは、値を解釈せず `observation_reason_forbidden` を返す。

成功は次である。

```json
{"id":"...","kind":"observation","domain":"/canonical/path","content_sha256":"..."}
```

### believe

信念を一つ作る。埋め込む文字列は `text` だけである。

```json
{
  "text": "二つの記録は矛盾している",
  "reason": {"kind": "observation", "id": "..."},
  "about": ["...", "..."],
  "interval": null
}
```

`reason` を省略するか `null` のときは、理由なしである。あるときは次のいずれか一つである。

| `kind` | 必須 | 置いてはいけない鍵 |
| --- | --- | --- |
| `belief` | `id` | `text` |
| `observation` | `id` | `text` |
| `text` | `text` | `id` |

`kind` がこの三つ以外なら `reason_kind_invalid` である。`id` の対象が無ければ `reason_not_found` である。理由にできるのは、解決ディレクトリの部分木に既にある信念または観測だけである。

`about` を省略したときは空である。要素は既存の信念または観測の識別子で、重複が無く、昇順で保存する。未知の識別子は `object_not_found` である。矛盾は、本文がその意味を持ち、`about` が対象を指す信念として表す。専用の関係種別は無い。

成功の形は観測と同じで、`kind` は `belief` である。

### relate

同一ドメインの順序を一つ結ぶ。本文は次だけである。

```json
{"kind": "next", "from": "...", "to": "..."}
```

`kind` が `next` 以外なら `relation_kind_invalid` である。両端が同じ、またはどちらかが無いときは `relation_endpoints_invalid` である。両端のドメインが違うときは `cross_domain_next` である。

```json
{"kind": "next", "from": "...", "to": "...", "domain": "/canonical/path"}
```

### search

自然文で、解決ディレクトリを根とする部分木を検索する。祖先と兄弟は含めない。

```json
{"query": "矛盾していないか", "limit": 8}
```

`limit` を省略したときは 8 である。1 以上 20 以下の整数でないときは `limit_invalid` である。

順位は、問いに対するコサイン類似度の降順、同点では識別子の昇順である。`limit` は信念と観測を合わせた件数である。返却時に種別ごとの配列へ分け、配列内の順序は順位を保つ。`score` は小数第 6 位までの十進である。

部分木に、現在のプロバイダ、base URL、モデルのキャッシュが無い記録があるときは、結果を返さず `embed_cache_missing` である。

観測の要素は `domain`、`id`、`interval`、`kind`、`reference`、`score`、`text` である。信念の要素は `about`、`domain`、`id`、`interval`、`kind`、`reason`、`score`、`text` である。源ファイルの本文と、Reference の周辺テキストは返さない。

```json
{"beliefs": [], "observations": []}
```

### domain-attach

所属の索引だけを変える。

```json
{"id": "...", "domain": "/absolute/canonical/path"}
```

`domain` は絶対パスである。正規化の結果が存在するディレクトリでない、またはルートであるときは `domain_invalid` である。対象が部分木に無ければ `object_not_found` である。成功時の `content_sha256` は、作成時に返した値と一致する。

```json
{"id": "...", "domain": "/absolute/canonical/path", "content_sha256": "..."}
```

### reindex

本文は `{}` である。部分木の全記録を、現在のプロバイダとモデルでキャッシュし直す。失敗したときは、その組の新しいキャッシュを残さない。以前の組のキャッシュは削除しない。

```json
{"provider": "fixture", "model": "fixture-model", "count": 0}
```

## 埋め込み

登録、検索、再索引は、プロバイダとモデルが揃っていることを前提にする。どちらかが無いときは `embed_provider_unset` である。接続できない、または 10 秒を超えたときは `embed_unreachable` である。応答にベクトルが無いときは `embed_rejected` である。同じ組の索引と次元が違うときは `embed_dimension_mismatch` である。

`fixture` のファイルは次の形だけである。`vectors` の鍵は、登録する本文または検索語と完全一致する。一致が無いとき、次元が `dimension` と違うときは `embed_fixture_invalid` である。

```json
{"model": "fixture-model", "dimension": 2, "vectors": {"発話の記録": [1.0, 0.0]}}
```

`ollama` が送る要求は `POST {base}/api/embed`、本文 `{"model":"...","input":"..."}` である。使うベクトルは応答の `embeddings` の先頭である。

`openai` が送る要求は `POST {base}/embeddings` である。`Authorization` は `Bearer` と、指定した環境変数の値である。変数が未設定または空なら `embed_provider_unset` である。本文は `{"model":"...","input":"..."}` である。使うベクトルは応答の `data` 先頭の `embedding` である。

契約テストは、この要求形と、記録した応答だけで判定する。受入セッションの結合は `fixture` だけを使う。

## 拒否コード

| code | 終了 | message |
| --- | --- | --- |
| `invalid_json` | 2 | 入力は1つの JSON オブジェクトである |
| `invalid_command` | 2 | 未知のコマンドである |
| `invalid_flag` | 2 | 未知のフラグがある |
| `unknown_field` | 2 | 未知のフィールドがある |
| `missing_field` | 2 | 必須フィールドがない |
| `invalid_type` | 2 | フィールドの型が契約と違う |
| `xdg_relative` | 1 | XDG のパスが相対パスである |
| `dir_not_found` | 1 | 解決ディレクトリが存在しない |
| `dir_not_directory` | 1 | 解決先がディレクトリではない |
| `dir_is_root` | 1 | 解決ディレクトリがファイルシステムのルートである |
| `text_empty` | 1 | 本文が空である |
| `text_too_long` | 1 | 本文が8192文字を超える |
| `interval_invalid` | 1 | 期間の形式が契約と違う |
| `observation_reason_forbidden` | 1 | 観測の理由は受け付けない |
| `reason_kind_invalid` | 1 | 信念の理由種別が契約にない |
| `reason_not_found` | 1 | 理由の対象が存在しない |
| `reference_escapes_domain` | 1 | Reference の源が解決ディレクトリの外である |
| `reference_not_found` | 1 | Reference の源ファイルが存在しない |
| `reference_encoding` | 1 | Reference の源が UTF-8 ではない |
| `reference_span_invalid` | 1 | Reference の範囲が源の外である |
| `relation_kind_invalid` | 1 | 順序の種別が契約にない |
| `relation_endpoints_invalid` | 1 | 順序の両端が使えない |
| `cross_domain_next` | 1 | 次発話は同一ドメインに限る |
| `object_not_found` | 1 | 対象が存在しない |
| `limit_invalid` | 1 | 検索上限が 1 以上 20 以下ではない |
| `domain_invalid` | 1 | 付け替え先が正規化できるディレクトリではない |
| `embed_provider_unset` | 1 | 埋め込みプロバイダまたはモデルが未設定である |
| `embed_unreachable` | 1 | 埋め込みプロバイダへ接続できない |
| `embed_rejected` | 1 | 埋め込みプロバイダがベクトルを返さなかった |
| `embed_dimension_mismatch` | 1 | 埋め込みの次元が索引と一致しない |
| `embed_cache_missing` | 1 | 検索対象の埋め込みキャッシュが欠けている |
| `embed_fixture_invalid` | 1 | フィクスチャのベクトル定義が契約と違う |

## 受入セッションが確認する列

一時ディレクトリと一時の `XDG_DATA_HOME` で、プロバイダは `fixture` とする。

1. `init` が解決ディレクトリのドメインを返す。
2. ファイルのスパンを `reference` とする `observe` が成功する。
3. `observe` に `reason` として信念を置くと `observation_reason_forbidden` になり、オブジェクトは増えない。
4. `believe` が、理由なし、信念、観測、自然文の四つで成功する。
5. 二つの対象を `about` に持つ信念を登録できる。
6. 同一ドメインの `relate` が成功し、別ドメインの `relate` は `cross_domain_next` になる。
7. 与えた期間を `search` が同じ文字列で返す。
8. 登録本文と語が重ならない `query` でも、対応する記録がスライスに入る。スライスに源ファイル全体は無い。
9. 子ディレクトリの `--dir` では親の記録を返さず、親からの検索は子を含む。
10. `domain-attach` の前後で `content_sha256` が一致する。
11. `--dir /` は `dir_is_root`、相対パスの `XDG_DATA_HOME` は `xdg_relative` である。
