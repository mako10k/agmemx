# agmemx v7 CLI 契約候補

状態: Candidate（2026-09-29）。[Accepted v7 要件](requirements-crud-associative-v7-acceptance.md)、[Accepted v7 基本設計](architecture-v7-acceptance.md)、[v7 詳細設計候補](detailed-design-v7-candidate.md) を入力とする公開境界の提案である。旧 [cli-contract.md](cli-contract.md)・[cli-surface.md](cli-surface.md) の互換は要求しない。正式な受入前に実装の公開仕様として扱わない。

## 1. 共通の呼出し規則

実行ファイルは `agmemx`。操作は `agmemx [--dir=PATH] [--format=text|json] GROUP VERB [入力]` の形にする。全コマンドの操作語は `create|get|list|correct|delete` を同じ意味で使う。`create` は新 ID、`correct` は同じ ID の訂正、`delete` は同じ ID の訂正としての論理削除を指す。履歴上の `change|end` は `record` だけに別操作として置く。`domain` は CRUD 対象ではなく `attach|move` のみとする。`--dir` を省略した場合は起動時 cwd を canonical path に解決する。全体フラグはコマンド前、操作フラグは後に置き、値は `--name=value` または `--name value` を受け付ける。以下の構造化フィールド以外は引数経路で表せる。

`search` の埋め込み環境は、コマンド前の全体フラグ `--embed-provider=ollama|openai`、`--embed-model=NAME`、`--embed-base-url=URL`、任意の `--embed-api-key-env=ENV_NAME` で選ぶ。同名の環境変数はそれぞれ `AGMEMX_EMBED_PROVIDER`、`AGMEMX_EMBED_MODEL`、`AGMEMX_EMBED_BASE_URL`、`AGMEMX_EMBED_API_KEY_ENV` とし、各フィールドでフラグを環境変数より優先する。既定のプロバイダ・モデル・base URL は置かず、最初の3値がそろった場合だけ有効なプロファイルとする。base URL は認証情報・query・fragment を含めず、`embed` の正規化規則で key を作る。OpenAI の key 環境変数名は未指定なら `OPENAI_API_KEY` とし、Ollama には key を要求しない。key の**値**を CLI 引数、JSON、保存データ、ログに含めず、指定された環境変数から読む。未設定・不完全・接続不能なプロファイルでも記録・関係・所属操作には影響せず、検索方式の選択だけに影響する。これらのフラグは `search` 以外の CLI 操作では受け付けない。

`--input-json=-` は標準入力から、`--input-json=PATH` は UTF-8 ファイルから、当該操作の JSON オブジェクトを一つ読む。この指定がある呼出しで操作フラグまたは位置引数が一つでもあれば `mixed_input` で全体を拒否する。`--dir` と `--format`、`search` の `--embed-*` は全体フラグなので JSON 入力と併用でき、埋め込み設定を JSON の操作フィールドには含めない。JSON が標準入力に存在するだけでは自動的に読まない。JSON 入力のフィールド名・意味は下表の引数経路と同じであり、どちらも同じ型付き要求に変換する。未知フィールド、同名 JSON フィールドの重複、未知フラグ、欠落、型違い、余分な位置引数は拒否し、変更は行わない。複数値は `--about-id=ID` などの反復フラグで渡し、JSON では配列にする。入力ファイルそのものは変更しない。

`--format` の既定は TTY なら `text`、それ以外なら `json`。JSON は成功時 `{"ok":true,"result":{...}}`、拒否時 `{"ok":false,"error":{"code":"...","message":"..."}}` の**一つの**オブジェクトと改行を stdout に書く。text 成功は stdout、拒否は stderr に `error: ...` と `help: agmemx help GROUP VERB` を出す。処理成功は exit 0、入力・対象・競合・埋め込み利用不能・予算不足は exit 1、保存 I/O または確定結果不明は exit 2 とする。API key、記憶本文、Reference 文脈は拒否文やログへ埋め込まない。`schema` は操作ごとの入力・成功・拒否の JSON schema とフラグ対応を機械向けに出す。

## 2. 型と共通フィールド

`RecordID` と `RelationID` は小文字16進32文字であり、作成時にシステムが割り当て、再利用しない。`BodyHash` は小文字16進64文字。`kind` は記録で `belief|observation`、公開関係で `next|change|end`。`text` は空でない UTF-8 自然文、`display_name` は省略可能な文字列。`interval` は `{"start":"RFC3339","end":"RFC3339"}` または `null` で、時刻の順序を検証するが期間の意味を決めない。

外部 `Reference` の入力は `{"source":"相対path","start":0,"end":4}` で、範囲は Unicode scalar の半開区間とする。記録の `source` は登録・訂正時の全体フラグ `--dir` から得た `reference_base_domain` を基準に解決し、所属だけの付け替えでは基準を変えない。関係の `evidence_reference.source` も端点の所属と独立して、登録・訂正時の `--dir` から得た `evidence_base_domain` を基準に解決する。どちらの基準 path も取得時に返す。所属外へ逃げる path、存在しない源、不正 UTF-8、範囲外を拒否する。取得・検索では元の源と範囲を保持し、後から解決できない場合は `resolution:"unresolved"` とする。`reason` は `null`、`{"kind":"belief|observation","id":"..."}`、または `{"kind":"text","text":"..."}`。信念では任意、観測の直接の理由は `reference` であり、信念理由は受け付けない。信念の直接 `reference` と観測の `reason` はどちらの入力経路でも拒否する。`about` は重複しない記録 ID の配列。矛盾は `about` 付きの信念で表す。

関係の `reason` は `null`、自然文 `{"kind":"text","text":"..."}`、記録参照 `{"kind":"record","id":"..."}` のいずれか。`evidence_reference` は外部 `Reference` または `null`。両方とも省略可能で、指定した値は関係に保持する。関係の訂正削除や端点記録の訂正削除は、登録時の履歴を物理消去したことを意味しない。`next`、`change`、`end` を混同せず表示する。

## 3. 操作目録

以下の `--field` は引数経路の名前。JSON 経路の正確なフィールドは表の「JSON 入力フィールド」列に従う。分割した `--ref-*`、`--interval-*`、`--reason-*` は、それぞれ JSON の `reference`、`interval`、`reason` オブジェクトに組み立てる。`--link-*` は `link_reason` と `link_evidence_reference` に組み立て、`record change/end` の新記録用フラグは `new_record` に入れる。単一値の `--from-id` などはハイフンをアンダースコアに変えたフィールドに対応する。`--id` の代わりに `get|correct|delete` は直後の位置引数 `ID` を使えるが、両方を同時に置けない。`list` と `search` の範囲は `--dir` の部分木である。`--limit` は正の整数、既定20。入力の意味制約は両経路で同じである。

| 操作 | 引数経路の入力 | JSON 入力フィールド | 成功 `result` の必須フィールド |
| --- | --- | --- | --- |
| `record create` | `--kind`, `--text`, 任意の `--display-name`, `--interval-start`, `--interval-end`, `--ref-source`, `--ref-start`, `--ref-end`, `--reason-kind`, `--reason-id` または `--reason-text`, 反復 `--about-id` | `kind,text,display_name?,interval?,reference?,reason?,about?` | `record` |
| `record get ID` | `ID` または `--id` | `id` | `record` |
| `record list` | 任意の `--kind`, `--limit` | `kind?,limit?` | `records,total` |
| `record correct ID` | `ID` または `--id` と変更するフィールド。空の訂正は拒否 | `id` と変更する `text?,display_name?,interval?,reference?,reason?,about?` | `record` |
| `record delete ID` | `ID` または `--id` | `id` | `id,deleted,cascaded_relations,cascaded_internal_refs` |
| `record change` | `--from-id`, 新記録用の `record create` と同じフィールド、任意の `--link-reason-text` または `--link-reason-id`、`--link-ref-source`, `--link-ref-start`, `--link-ref-end` | `from_id,new_record,link_reason?,link_evidence_reference?` | `old_id,new_record,relation` |
| `record end` | `record change` と同じ | `from_id,new_record,link_reason?,link_evidence_reference?` | `old_id,new_record,relation` |
| `relation create` | `--kind`, `--from-id`, `--to-id`, 任意の `--reason-text` または `--reason-id`, `--ref-source`, `--ref-start`, `--ref-end` | `kind,from_id,to_id,reason?,evidence_reference?` | `relation` |
| `relation get ID` | `ID` または `--id` | `id` | `relation` |
| `relation list` | 任意の `--endpoint-id`, `--direction=incoming|outgoing|both`, `--kind`, `--limit` | `endpoint_id?,direction?,kind?,limit?` | `relations,total` |
| `relation correct ID` | `ID` または `--id` と訂正する `--kind`, `--from-id`, `--to-id`, 理由・根拠。空の訂正は拒否 | `id` と変更する `kind?,from_id?,to_id?,reason?,evidence_reference?` | `relation` |
| `relation delete ID` | `ID` または `--id` | `id` | `id,deleted` |
| `domain attach` | `--id`, `--to-dir` | `id,to_dir` | `id,domain` |
| `domain move` | `--from-dir`, `--to-dir` | `from_dir,to_dir` | `from_dir,to_dir,moved_count` |
| `search` | `QUERY` または `--query`, 任意の `--mode=auto|embedding|text`, `--budget`, `--limit` | `query,mode?,budget?,limit?` | `mode,total,candidates,omitted,budget_used` |

`record correct` の JSON ではフィールド欠落は「変更しない」、nullable な `display_name,interval,reason` の明示 `null` は「消す」を意味する。観測の `reference` は必須であり、信念には直接 `reference` を置かないため、`reference:null` は常に拒否する。反復 `--about-id` は集合全体の置換、`--clear-about` は空集合への置換であり、同時指定を拒否する。`--clear-display-name`、`--clear-interval`、`--clear-reason` はそれぞれ nullable 値の `null` に対応する。理由・Reference の分割フラグは全要素が揃ったときだけ一つの値に組み立てる。`record change/end` の `new_record` は CLI 引数経路では create 用フラグから作る。`record change/end` の link reason と新記録自身の reason は異なるスロットである。

`relation correct` は関係 ID を維持し、`kind`、端点、理由、根拠の指定された値だけを訂正する。`--clear-reason`、`--clear-evidence` は JSON の明示 `null` に対応する。変化・終了の関係を訂正しても旧記録を論理削除せず、関係の端点と種類の制約を再検証する。`relation create` で既存の二記録を結ぶことはできるが、同時に新記録を作る操作は `record change/end` に限る。

`relation create/correct` の `--to-id` 候補提示は、`--from-id` の生存記録と**同じ canonical domain** に所属する生存 ID を既定とする。`record change/end` の `--from-id` 候補提示は、新記録の登録先 `--dir` と同じ domain の生存 ID を既定とする。内部参照 `--about-id`、記録の `--reason-id`、関係の `--reason-id` と `--link-reason-id` の候補も同じ規則で提示する。既存記録の訂正ではその記録の現在の domain、新記録の作成では登録先 `--dir`、関係理由では関係の始点記録の現在の domain を基準とする。候補提示では記憶本文・Reference 文脈を読まない。候補外の ID を引数または JSON で明示したときも、対象が生存しているなど通常の型・参照制約を満たせばクロスドメイン Link を登録・訂正できる。公開関係と内部参照のいずれにも現在の両端ドメイン一致検査を置かない。候補提示と登録許可は別の契約であり、将来の許可／禁止の切替は現在のコマンドや設定へ追加しない。

`domain move --from-dir` は保存されている旧 canonical prefix の文字列であり、旧ディレクトリがもう存在しなくても受け付ける。`--to-dir` は現在存在する新ディレクトリを canonical path に解決する。対象部分木の記録所属と、同じ範囲にある記録 `reference_base_domain`・関係 `evidence_base_domain` を一括で更新し、記録 ID・本文 hash・関係端点は維持する。`domain attach` は単一記録の所属を変え、外部 Reference の独立した基準 path は変更しない。`search --mode=auto` は全件の埋め込みが使えなければ対象全体を全文方式にする。`--mode=embedding` は使えない場合に `embedding_unavailable` とする。対象範囲が空でも `--mode=embedding` は問い合わせ vector の取得を要し、設定不備・取得失敗なら同じエラーを返す。空範囲の `auto|text` はプロバイダを呼ばず、空の全文結果を返す。`--mode=text` は明示的な全文検索である。

## 4. 応答の厳密な形

`record` は `id,kind,text,display_name,domain,body_hash,interval,reference,reference_base_domain,reason,about` を持ち、nullable 値は省略せず `null` を返す。`reference` が無ければ `reference_base_domain` も `null`。通常の `get/list/search` に論理削除済み記録は含めない。`relation` は `id,kind,from_id,to_id,reason,evidence_reference,evidence_base_domain` を持つ。`evidence_reference` が無ければ `evidence_base_domain` も `null`。通常の関係取得・一覧には論理削除済みを含めない。`total` は予算切詰め前の一致件数、`records` と `relations` は ID と必要な本文／関係値を含む配列である。`record list` と `relation list` は `total` と返却件数を区別する。

| 型 | key の型 |
| --- | --- |
| `Record` | `id:RecordID`, `kind:belief|observation`, `text:string`, `display_name:string|null`, `domain:DomainPath`, `body_hash:BodyHash`, `interval:Interval|null`, `reference:ResolvedReference|null`, `reference_base_domain:DomainPath|null`, `reason:RecordReason|null`, `about:RecordID[]` |
| `Relation` | `id:RelationID`, `kind:next|change|end`, `from_id:RecordID`, `to_id:RecordID`, `reason:RelationReason|null`, `evidence_reference:ResolvedReference|null`, `evidence_base_domain:DomainPath|null` |
| `ResolvedReference` | `source:string`, `start:integer`, `end:integer`, `resolution:resolved|unresolved`, `unresolved_reason:string|null`, `context:string|null`。`context` は解決成功かつ予算で採用された場合だけ文字列 |
| `RecordReason` | `kind:belief|observation` と `id:RecordID`、または `kind:text` と `text:string` |
| `RelationReason` | `kind:record` と `id:RecordID`、または `kind:text` と `text:string` |

観測の `reference,reference_base_domain` は必ず非 null、信念では必ず null とする。`get/list` の `ResolvedReference.context` は解決可能な場合に限り返し、解決不能でも記録を隠さない。各操作の `result` は上表の型に従い、`record` は `Record`、`relation` は `Relation`、`records` は `Record[]`、`relations` は `Relation[]`。`cascaded_relations,cascaded_internal_refs,moved_count,total` は非負整数、`old_id,id` は `RecordID`、`new_record` は `Record`、`deleted` は真偽値、`domain,from_dir,to_dir` は `DomainPath` とする。成功結果の追加 key は出さない。

検索の各 `candidate` は `record_id,kind,score,score_kind,summary,stats,hints` を持つ。`score_kind` は `cosine|lexical`。`stats.incoming_references` は他の生存記録・関係からこの候補を指す `internal_refs` 本数の非負整数、`stats.relation_counts` は `next|change|end` ごとに `incoming,outgoing` の非負整数を持つオブジェクト、`stats.external_reference_state` は `none|resolved|unresolved` である。各 `hint` は `source_kind:"relation|reason|about"`、`record_id`（自然文だけの理由では `null`）、`relation_id`（公開関係以外では `null`）、`relation_kind`（公開関係では `next|change|end`、それ以外では `null`）、`direction:"incoming|outgoing"`、`domain`（記録 ID がない場合は `null`）、`reason`（`RelationReason|null`。信念の記録理由を表示するときも、記録参照は `kind:record` に正規化する）、`evidence_reference`（`ResolvedReference|null`）、`evidence_base_domain`（外部根拠が無い場合は `null`）、`reason_present,evidence_present`（真偽）、`summary`（`string|null`）を持つ。候補自身の順位・一致件数に hint を加えない。`mode` は実際の `embedding|text`。`omitted` は `candidates,hints,summaries` の省略数と `truncated` 真偽を持つ。Reference は元の `source,start,end` と `resolution:"resolved|unresolved"`、解決不能なら理由コードを保持する。予算の計測範囲は成功オブジェクト全体の UTF-8 byte 数であり、`budget_used` は自分自身を含めて最終直列化した byte 数と一致するまで再計算する。text 形式も実際の表示 byte 数が予算以内である。

JSON は上で列挙した key だけを持ち、入力では未知 key を拒否する。成功時の配列は必ず配列、nullable 値は必ず `null` または指定型、数値は整数または有限の score とする。`schema` が出す機械向け定義をこの文書の表と型から生成し、手書きの別仕様を持たない。対象が無いオプション値は入力で省略できるが、成功応答の nullable key は省略しない。`total`、`budget_used`、各省略数は非負整数で、`score` は有限の数値、`deleted` と `truncated` は真偽値とする。拒否 `code` と固定 `message` は次表による。理由の詳細や path は機密を含め得るため返さない。

| code | message |
| --- | --- |
| `invalid_input` | 入力の値または組合せが不正です |
| `missing_field` | 必須の入力がありません |
| `unknown_field` | 未定義の入力があります |
| `invalid_type` | 入力の型が違います |
| `mixed_input` | 引数と JSON 入力を同時に指定できません |
| `not_found` | 対象が見つかりません |
| `deleted` | 対象は論理削除されています |
| `conflict` | 状態が変わったため操作を確定できません |
| `storage_failure` | 記憶庫を読み書きできません |
| `commit_unknown` | 確定結果を確認できません |
| `embedding_unavailable` | 埋め込み検索を利用できません |
| `budget_too_small` | 出力予算が最小結果より小さいです |

## 5. 利用者向け目録と worker

`agmemx help [GROUP [VERB]]`、`agmemx schema [GROUP [VERB]]`、`agmemx __completion --bash`、`agmemx --help` は同じ操作定義を読む。Bash 補完は薄いラッパーとし、記憶本文・API key・プロバイダへアクセスしない。man は同じ目録から生成し、操作とオプションの有無が help・schema・補完と一致する。コマンド名は Git または Docker の同名主要操作と逆の意味にしない。

別実行ファイル `agmemx-embed-worker` は `agmemx-embed-worker [--embed-provider=...] [--embed-model=...] [--embed-base-url=...] [--embed-api-key-env=...] run|once|status` を持つ。`run|once` は §1 と同じフラグ名・環境変数・優先順位・正規化でプロファイルを選び、未設定・不完全なら `invalid_input` で開始しない。`status` はプロファイル不要で、処理待ち・失敗・成功件数を返し、本文や鍵を返さない。worker と検索のプロファイル key が異なると検索はその worker の成果を使わず、`auto` は全文方式、明示 `embedding` は `embedding_unavailable` となる。worker の失敗は書込み成功の巻き戻しではない。CLI の公開 CRUD に埋め込み設定 CRUD は置かない。実装の起動・再試行は詳細設計の worker 契約に従う。

本候補は v7 詳細設計候補と同時にレビューし、受入後に現行 CLI 文書・help・schema・補完・man と計画へ反映する。旧 CLI 形、旧データの移行、互換エイリアスを追加しない。
