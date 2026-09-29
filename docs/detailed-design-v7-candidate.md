# agmemx v7 詳細設計候補

状態: Candidate（2026-09-29）。所有者の受入、現行設計への反映、実装適合を意味しない。

## 1. 適用範囲と設計の境界

根拠は [Accepted v7 要件](requirements-crud-associative-v7-acceptance.md) が固定した [要件本文](requirements-crud-associative-v7-candidate.md) と、[Accepted v7 基本設計](architecture-v7-acceptance.md) が固定した [基本設計本文](architecture-v7-candidate.md) である。本文 SHA-256 は順に `71f3d9b43008ade1365a3bd0b067129cd12b27bf4b9d785c247659aa06e364bf`、`c164bfbcd983e1d69551a01aa3c31af07b3cf51865df4b988930bdf029455247`。この文書は基本設計 §「詳細設計へ渡す未確定事項」を具体化する候補であり、旧 [architecture.md](architecture.md) や [cli-contract.md](cli-contract.md) の衝突する規定を根拠にしない。

対象はローカルの記憶庫、13 個の一対一 Go module、CLI と埋め込み worker の別プロセス、各モジュール間の契約である。公開コマンド・入力・応答の正確な形は別の [v7 CLI 契約候補](cli-contract-v7-candidate.md) に置く。両候補は一緒にレビューする。旧保存形式の読取・変換、物理パージ GC、期間の意味の再定義、矛盾専用型は含めない。

## 2. module と依存

リポジトリの `modules/<name>/go.mod` を独立した単位とし、module path は `github.com/mako10k/agmemx/modules/<name>` とする。ルート `go.work` に13個を列挙してローカルで接続する。現行ルート `go.mod` と `internal/` は v7 実装で置き換える対象であり、互換のために残す契約は置かない。公開 package は各 module のルートに一つ、実装詳細は当該 module の `internal/` に閉じる。CLI module の `cmd/agmemx`、embed module の `cmd/agmemx-embed-worker` が別の実行入口である。

| module | 公開 package と所有する変更理由 | 直接参照できる下位 module | 利用側が持つ主要 port |
| --- | --- | --- | --- |
| `model` | `model`: 記録・関係・理由・Reference の値と意味上の検証 | なし | なし |
| `domain` | `domain`: canonical path、部分木、移動対応付け | なし | なし |
| `reference` | `reference`: 外部源のスパンを安全に解決 | `model`, `domain` | `SourceReader` |
| `body` | `body`: 本文バイトの内容アドレス保管 | `model` の hash 型のみ | `BodyStore` は利用側に定義 |
| `ledger` | `ledger`: 一貫した読取像と原子的状態確定 | `model`, `domain` | `SnapshotReader`, `Committer`, `Queue` は利用側に定義 |
| `embed` | `embed`: 別プロセスの埋め込み生成とベクトル成果 | `model` の hash 型のみ | `ActiveBodies`, `BodyReader`, `VectorWriter` |
| `candidate` | `candidate`: 埋め込み／全文の方式選択と候補順位 | `model`, `domain` | `ScopedReader`, `LexicalReader`, `VectorReader`, `QueryEmbedder` |
| `association` | `association`: 関係の手掛かり、統計、予算内表現 | `model`, `domain` | `GraphReader`, `BodyReader`, `ReferenceResolver` |
| `record` | `record`: 記録の作成・訂正・変化・終了・訂正削除 | `model`, `domain` | `BodyStore`, `SnapshotReader`, `Committer` |
| `relation` | `relation`: 単独関係の作成・訂正・訂正削除 | `model` | `SnapshotReader`, `Committer` |
| `membership` | `membership`: 記録所属変更と部分木移動 | `model`, `domain` | `SnapshotReader`, `Committer` |
| `read` | `read`: ID 取得・一覧・検索の一つの読取操作 | `model`, `domain`, `candidate`, `association` | `SnapshotReader`, `BodyReader` |
| `cli` | `cli`: 入力経路、表示、公開目録、実行入口の組立て | 上記12 module の公開 API のみ | なし |

Go の interface は原則として呼出し側 module に置き、提供側の具象型はそれを構造的に満たす。例外は `model` の不変値型だけとする。`record`、`relation`、`membership`、`read`、`candidate`、`association` は `ledger/internal`、SQL、HTTP クライアントを import しない。`cli/cmd/agmemx` と `embed/cmd/agmemx-embed-worker` の組立てコードだけが提供側の公開コンストラクタを知る。共有された「便利な業務処理」module は作らない。公開 API と import graph を静的チェックし、禁止された実装 package への import と循環依存を拒否する。新規・実質変更のソース module には実際の単独責務を記した `R: <Responsibility>` module コメントを置く。

### 2.1 値と port の契約

`RecordID`、`RelationID` は生成された128 bit の不透明 ID、`BodyHash` は後述の SHA-256、`DomainPath` は正規化絶対 path、`Revision` は台帳全体の単調増加整数とする。公開 port は Go の `context.Context` を第一引数に取り、キャンセルは未確定操作を取り消す。port は保存表や HTTP 応答を公開しない。

`ReadSnapshot(ctx)` は一つの revision に属する生存・削除済み記録、関係、内部参照、所属、本文 hash と検索語索引への一貫した読取 handle を返す。handle を閉じるまで同じ revision を読む。通常の取得・一覧・検索用 API は削除済みを除く。内部の cascade 検査には削除状態も読める別 API を使う。`CommitChange(ctx, expectedRevision, delta)` は全変更と queue 行を一つのトランザクションで適用し、新 revision または型付き拒否を返す。`delta` は記録・関係・内部参照・所属・検索語・queue の明示的な差分であり、台帳は意味の推測をしない。

`record` が訂正削除を作る際は読取像で対象 ID に接続する入出関係と内部参照を列挙する。`ledger` は確定時にその ID を参照する全生存リンクと `delta` を再照合する。過不足、revision 不一致、制約違反は全件を拒否する。`record` が選ぶ削除対象と、`ledger` が保証する原子性を分ける。

## 3. 物理データと確定

配置は `${XDG_DATA_HOME:-$HOME/.local/share}/agmemx/` の `bodies/sha256/<先頭2桁>/<残り62桁>` と `state.sqlite`、`${XDG_CACHE_HOME:-$HOME/.cache}/agmemx/vectors.sqlite` とする。相対 XDG root は拒否する。状態 DB は SQLite の一つのファイルを唯一の原子的確定境界とし、同一ホストのローカルファイルシステム上で WAL、`foreign_keys=ON`、`synchronous=FULL`、有限の busy timeout を設定する。WAL を共有できない配置は起動時に拒否する。CLI と worker は同じ DB を開くが、各書込みは短いトランザクションで行う。別 DB のベクトル成果は記録の確定条件に入れない。この配置の同一ホスト制約と読取 snapshot は [SQLite の WAL](https://www.sqlite.org/wal.html) と [分離の説明](https://www.sqlite.org/isolation.html) を前提にする。

本文 hash は `SHA-256(UTF-8 の text 値のバイト列)` の小文字16進表記とし、前後の空白、改行、Unicode 正規化を暗黙に変更しない。表示名、ID、所属、理由、期間、Reference は hash に含めない。本文は有効な UTF-8 とし、内容を検証する。新しい保存先ディレクトリを作った場合は、各新規ディレクトリについて親を順に sync して作成を永続化する。同じ保存先ディレクトリに一時ファイルを書き、ファイルの flush・sync、既存実体を上書きしない原子的な確定、保存先ディレクトリの sync をすべて成功させてから台帳を確定する。既存 hash の実体は内容一致を検証し、不一致・hash 衝突は拒否する。既存実体を再利用するときも保存先ディレクトリを sync してから台帳を確定する。ディレクトリ sync を含む本文保存が失敗したら DB の確定に進まない。本文書込み後の DB 失敗では未参照 blob が残り得るが、読取像・検索・queue には現れない。物理 GC は行わない。

`state.sqlite` の論理表は次とする。列の細かな SQL 型と索引名は実装内に閉じるが、キー・参照・状態遷移は契約である。

| 表 | 主キーと必須要素 | 目的 |
| --- | --- | --- |
| `records` | `record_id`, `kind`, `body_hash`, `display_name`, `interval`, `reason_text`, `external_reference`, `reference_base_domain`, `domain_path`, `deleted_at_revision` | 現行本文、信念の自然文理由、削除状態。削除後も ID 行を残す |
| `relations` | `relation_id`, `kind`, `from_id`, `to_id`, `reason`, `evidence_reference`, `evidence_base_domain`, `deleted_at_revision` | `next`、変化、終了の有向関係。端点は記録 ID |
| `internal_refs` | `(owner_kind, owner_id, slot, target_id)`, `deleted_at_revision` | `about`、記録理由、関係理由の記憶ノード参照を列挙可能にする |
| `lexemes` | `(record_id, lexeme)`, `frequency` | 生存本文・表示名の全文照会。記録変更と同じ確定単位で更新 |
| `work_items` | `(body_hash, profile_key)`, `status`, `lease_until`, `attempt`, `next_at`, `last_error_class` | worker の再試行可能な要求。`profile_key='*'` は環境未指定で発見された本文を表す |
| `meta` | 単一行の `revision`, `schema_version` | 同時実行検査と破壊的 v7 形式の識別 |

各記録の所属は `records.domain_path` を正とし、別の所属コピーを正本にしない。関係・内部参照は端点と削除状態を外部キーおよび確定時検査で保つ。信念の自然文理由は `records.reason_text` に、信念・観測の ID を指す信念理由は `internal_refs` の記録理由スロットに置く。同じ生存記録で自然文理由と生存する記録理由リンクを同時に持たせず、観測は `reason_text` も記録理由リンクも持たない。読取像は生存する記録理由リンク、自然文理由、理由なしのいずれか一つから信念の `RecordReason` を復元する。理由の訂正・消去は本文ハッシュを変えずに両保存先を同じ DB commit で切り替える。記録の `Reference` は登録または訂正時の `--dir` を canonical path にした `reference_base_domain` を保持し、後から所属だけを付け替えても源を変えない。所属を持たない関係の `evidence_reference` も、登録または訂正時の `--dir` を canonical path にした `evidence_base_domain` を独立して保持し、端点どちらかの所属から推測しない。外部源・範囲を記録または関係に保持し、源ファイルを DB に複製しない。関係理由の自然文は関係行に、理由が別記録を指す場合は `internal_refs` に置く。保存時刻と期間は別列で、期間が何を指すかをこの設計は決めない。

一つの変更は、(1) 利用側が読取像で検証して delta を作成、(2) 必要な新本文を blob に保存、(3) `BEGIN IMMEDIATE` 相当で revision と全対象を再検査、(4) 記録・関係・内部参照・所属・語索引・必要な work item を適用、(5) revision を増やして commit、の順に行う。revision は記憶状態の変更だけで増やし、worker の lease 更新では増やさない。競合時は `Conflict` を返し、利用側が新しい読取像で変更意図から作り直す。古い delta の自動再適用はしない。DB commit の結果が不明な I/O 障害では呼出し側へ不明状態を返し、既知の ID と revision を再読取して判定する。新規作成で結果を判別できない場合も、非冪等な create を盲目的に再送しない。

## 4. 変更操作の状態遷移

`record.create` は `model` の型制約と必要な直接 Reference を検証し、新 ID、本文、所属、内部参照、語索引、埋め込み待ちを確定する。観測は発話・文書スパンの直接 Reference を持ち、信念の理由は省略可能で信念・観測・自然文に限る。矛盾専用の状態や辺は作らない。`record.correct` は同じ ID の現行本文 hash／メタデータだけを訂正する。本文が変わったときだけ新 hash の待ちを追加し、既存の無関係な関係と参照を保つ。理由・`about` の訂正はそのスロットだけを変更する。

`record.change` と `record.end` は旧記録を生存させたまま新 ID の信念または観測と `change` または `end` の有向関係を**旧 ID → 新 ID**で同時確定する。終了を別の新しい記録種別にせず、新記録の本文と `end` 関係で表す。旧記録の本文と所属は変更しない。関係は理由の自然文または参照可能な記録 ID と、根拠となる任意の外部 Reference を別スロットで持てる。利用者が指定した理由・根拠を失わず、検索では関係種類と方向を別に表示する。単独の `relation.create/correct/delete` でも公開関係の ID を扱えるが、`record.change/end` の原子的な組を分割しない。

**所有者決定（2026-09-29、Link のドメイン）:** 公開関係 `next`・`change`・`end` と内部参照 `about`・記録理由・関係理由は、接続先との現在のドメイン一致を確定時の検証条件にしない。公開関係の端点選択候補は始点の現在の canonical path と**同じドメイン**の生存記録を既定で提示し、`record.change/end` の旧記録候補は新記録の登録先 `--dir` と同じドメインから提示する。内部参照の候補も所有記録の現在のドメインから提示する。新記録を作る操作では登録先 `--dir` を用い、関係理由では関係の始点のドメインを用いる。候補一覧は発見を助けるだけで、型・生存などの制約を満たす ID を明示した場合は候補外・クロスドメインでも受け付ける。`next` は順序を表す関係種類であり、両端の現在の所属一致を存続条件としない。所属の付け替えやディレクトリ移動後も、端点 ID と内部参照を含む Link 自体を保持する。将来、クロスドメイン Link の許可・禁止を選べる機能は別の要件・設計で扱い、v7 にポリシー切替や禁止モードを加えない。

`record.delete` は誤登録の訂正である。対象の生存記録、入出両方向の全公開関係、`about`・記録理由・関係理由にある全内部参照リンクを同じ commit で論理削除する。残る信念の `about` 要素は削除し、削除対象を指していた任意の記録理由・関係理由の ID スロットは空にする。外部 Reference の源ファイルは変更しない。参照されていたことだけで削除を拒否しない。対象の関係と内部リンクに漏れがあれば全体を拒否する。論理削除済みの ID は再利用しない。`relation.delete` は指定関係だけを論理削除し、端点の記録は保持する。通常の ID 取得・一覧・検索と連想表示は削除済み記録・関係を出さない。後続 GC がない限り物理パージを主張しない。

`membership.attach` は指定記録の `domain_path` のみを変更し、ID・本文 hash・関係端点・`reference_base_domain` を維持する。`membership.move` は移動**後**に `from` の保存済み絶対 prefix と、実在する `to` の canonical path を受ける。`from` の元ディレクトリが既に無くてもよい。`from` と子孫に属する全記録の所属と、同じ範囲にある記録の `reference_base_domain`・関係の `evidence_base_domain` の suffix を `to` に写し、一つの revision で更新する。別記録が移動先に既に属する場合は共存させ、ID で区別する。対象集合は読取像で固定し、確定時に対象集合または revision が変われば競合として全体を拒否する。同じ prefix、ルート、空集合、不正 path は型付き拒否とする。記録と関係の `Reference.source` はそれぞれ独立した基準 path からの相対 path として保持し、移動後は更新した基準 path から解決する。旧 path との相対構造を変えない移動を対象とし、源ファイルの移動をこの操作で実行しない。

## 5. 別プロセス worker とベクトル

`agmemx-embed-worker run` は [v7 CLI 契約候補](cli-contract-v7-candidate.md) §1・§5 の共通プロファイル規則に従い、起動引数を環境変数より優先して一つの埋め込み環境を選び、明示的に停止されるまで work item を取得する。`once` は処理可能な項目を一巡して終了する。起動時と周期的に、現行の生存本文 hash と選択環境のベクトル成果を照合し、欠けている組を work item にする。未設定または不完全なプロファイルでは worker の `run|once` を開始せず、CLI の書込みはプロバイダ設定に依存せず確定できる。常駐起動の方法は利用者の環境に委ね、特定の service manager を必須にしない。CLI の一回の書込みが worker を同期実行しない。

成果の key は `(provider, normalized_base_url, model, body_hash)` とする。base URL は scheme・host・明示 port・path の有効部分を区別し、認証情報・query・fragment を含めず、末尾 slash のみ正規化する。`embed` は CLI 検索と worker が共用する公開プロファイル値・正規化処理を所有し、同じ入力から同じ `profile_key=(provider, normalized_base_url, model)` を作る。API key は指定された名前の環境変数から取得し DB・ログ・出力に書かない。`vectors.sqlite` は key、次元、有限数値の vector、作成時刻を保存する。異なる環境や次元の vector を混ぜない。本文 hash が訂正後に古くなっても、成果は cache として残り得るが、検索は読取像の現行 hash と完全一致するものだけ使う。

worker は `profile_key='*'` の発見対象と現行本文 hash を選択環境の実際の key へ展開する。DB の短いトランザクションで `(body_hash, profile_key)` を lease し、本文を取得して DB 外でプロバイダを呼び、ベクトルを key で冪等に保存した後、work item を成功にする。複数 worker の lease 期限切れによる重複計算は許すが、同じ key の結果だけを採用し、異なる次元・不正値は失敗として保持する。成功の記録前に停止しても、次の worker が成果の存在を読んで完了にできる。タイムアウト・一時的な接続失敗は `min(1時間, 2^attempt 秒)` の backoff に従い、入力・応答の恒久的な不正は失敗状態にして再構成または明示的な再試行まで待つ。lease は5分、呼出し timeout はその内側の30秒とし、停止シグナルでは新規 lease を止め、進行中の処理を期限内に終え、未完了項目は lease 満了後に再取得可能とする。worker は `records.body_hash`、ID、関係を変更しない。

## 6. 読取、検索、連想表示

`read.get/list` と `read.relationGet/list/byEndpoint` は一つの台帳読取像から生存対象を返す。検索に選ぶ埋め込みプロファイルは CLI の `--embed-*` 引数と `AGMEMX_EMBED_*` 環境変数を §5 の共通規則で解決したものとする。通常検索は同じ像の指定部分木にある生存記録を確定し、その**全件**について選択プロファイルの現本文 hash に対応する vector があり、問い合わせ vector も取得できる場合だけ cosine 類似度の埋め込み候補検索を使う。問い合わせのプロバイダ失敗、プロファイル未設定・不完全、どれか一件の vector 欠落では対象全体を全文方式へ切り替える。明示 `embedding` 方式は同条件を満たさなければ `embedding_unavailable` とし、全文に切り替えない。`text` 方式はプロファイルを参照しない。対象範囲が空なら `auto|text` はプロバイダを呼ばず空の全文結果を返す。明示 `embedding` は空範囲でも有効なプロファイルで問い合わせ vector の取得を試み、成功した場合だけ空の埋め込み結果を返す。設定不備または取得失敗なら `embedding_unavailable` とする。検索中に状態が変わっても同じ読取像の ID と hash を使う。

全文は `lexemes` の語一致であり、意味一致を主張しない。入力と本文・表示名を Unicode lower-case と幅の正規化後、空白／記号で区切った語と、連続する CJK 文字の重なり合う2文字片へ分ける。1文字 CJK 語も保持する。記録は少なくとも一語が一致したときだけ候補にする。異なる問い合わせ語数を `Q`、一致した語数を `M`、一致語の索引出現回数合計を `F` とし、全文 score は `M/Q + min(0.1, F/100)` とする。語が一つも得られない問い合わせは入力拒否とする。同点は ID 昇順とする。候補上限は順位に適用し、連想手掛かりへは適用しない。方式と score の尺度を返し、全文 score を埋め込みの意味類似度と表示しない。語索引は記録変更と同じ DB commit で更新し、削除済み記録は索引照会から除く。

`association` は候補の順番と一致件数を変えず、各候補から生存する `next`、`change`、`end` の入出関係、信念理由、`about` を一段だけ辿る。手掛かりは候補と区別し、端点 ID、種類、`incoming/outgoing`、理由・根拠の存在を持つ。A→B→C の B では A を前、C を後とする。候補自身または関係が削除済みなら辿らない。対象ドメイン外の手掛かりでも、到達した ID と所属を区別して表示する。候補のスコアが低いだけで接続手掛かりを除外しない。

統計は読取像の生存対象について、記録を指す内部参照数、公開関係の種類・入出方向別本数、外部 Reference の有無と解決状態を数える。Reference 解決は源と Unicode scalar 半開範囲を保持し、現在の源を読める場合だけスパン前後それぞれ40文字の文脈を得る。所属外へ解決されるシンボリックリンクは辿らない。欠落、範囲外、UTF-8 不正は `unresolved` として理由を付し、記録自体は残す。表示用要約は表示名と本文の先頭120文字、および解決できた文脈からの短い抜粋を切り出すだけで、新しい事実を生成しない。

出力予算は**最終的に直列化した UTF-8 バイト数**とし、既定 4096 byte とする。方式・一致総数・省略数と少なくとも先頭候補 ID を含む最小結果をまず確保する。残りは候補 score を方式内で0〜1へ正規化した値 `S`、内部参照数 `R`、関係本数 `L` から候補重み `100S + min(20,2R) + min(10,L)` を作って優先する。手掛かりの追加重みは `change/end` が20、`next` が10、`about/理由` が8とし、候補重みとの和を、その項目の直列化追加 byte 数で割った値の高い順に選ぶ。同点は候補順位、関係 ID または端点 ID 順とする。候補 ID と関係方向は対応する本文・文脈より先に収め、残りを本文抜粋・Reference 文脈に使う。同一手掛かり ID は候補ごとに重複表示しない。text と JSON の各形式で実際に serialize した byte 数を測り、上限まで安全な文字境界で切り詰め、`omitted` に件数と切詰めの事実を残す。最小結果さえ入らない予算は `budget_too_small` で拒否する。結果の同一性を受入条件に追加しない。

一致候補が0件なら最小結果は方式・一致総数・省略数だけでよく、存在しない先頭 ID を要求しない。

## 7. CLI との接続・失敗の所在

CLI は [v7 CLI 契約候補](cli-contract-v7-candidate.md) の操作目録から引数経路または明示 JSON 経路のどちらかを一つだけ parse し、型付き要求へ変換する。JSON／フラグ／text 表示は `cli` が所有し、`model`・ユースケースはそれを知らない。help、schema、補完、man は同じ目録データから生成する。`--dir` は呼出しの canonical path 解決に使い、domain の CRUD を作らない。CLI の入力や出力形式を変えるときは v7 CLI 契約候補と同時にレビューする。

拒否は `InvalidInput`、`NotFound`、`Deleted`、`Conflict`、`StorageFailure`、`EmbeddingUnavailable`、`BudgetTooSmall` の型付き分類に変換する。入力拒否と制約違反は変更前、競合と DB 障害は変更の全体拒否、commit 結果不明だけは `CommitUnknown` として再読取の案内を返す。ログには機密の API key、Reference の文脈全文、記憶本文を出さない。埋め込み失敗は書込みを巻き戻さず、worker の状態と検索方式にだけ影響する。

## 8. 検証の継ぎ目と未決定

モジュール単独では `model` の型制約、`domain` の prefix、`body` の hash バイト・新規ディレクトリ／rename 後 sync 失敗時の DB 未確定、`ledger` の競合と原子性、`record` の自然文／ID 理由の保存・復元・訂正と削除対象列挙、`embed` のプロファイル正規化・lease・重複・失敗、`candidate` の空範囲を含む方式切替、`association` の方向・統計・予算、`cli` の経路と目録一致を確認する。結合では一時 XDG 領域と実際の二プロセスを使い、Accepted v7 要件 §「受入の最低例」1〜7、同時訂正削除と追加リンクの競合、worker 停止後の再開、異なる検索／worker プロファイル時の全文切替と明示エラー、空範囲での明示埋め込み利用不能、部分木移動後の Reference 解決を観測する。代替品や mock を実能力の達成として数えない。

| Accepted 入力 | この候補で具体化した箇所 | 次の証拠 |
| --- | --- | --- |
| v7 要件 §「CRUD の対象と操作」「訂正と記憶の変化」 | §2〜4 の model・台帳・変更契約 | モジュール検証と受入例1〜3、6〜7 |
| v7 要件 §「CLI 入出力」 | §7 と v7 CLI 契約候補 | 引数／JSON の同等性と目録一致 |
| v7 要件 §「出力予算内の連想検索」 | §5〜6 の worker・候補・連想契約 | 全文切替、方向、統計、予算の結合検証 |
| v7 基本設計 §「責務と境界」「Go module の境界」 | §2 の13 module と port の依存 | import graph と単独検証 |
| v7 基本設計 §「保存と更新の確定境界」「実行単位の判断」 | §3〜5 の一つの台帳確定と別プロセス worker | 競合・失敗・再開を含む結合検証 |

この候補は製品上の未決定を勝手に解消しない。期間の意味は継承要件どおり未固定、物理パージ・復元は後続要件、旧保存形式・CLI の互換は不要である。[現行設計](architecture.md) の Cause メッセージ「1トークン」規則は v7 の実行時設計と衝突せず、次の登録にも適用する。既存 v7 の4件の文章メッセージはこの規則との差異として残っており、訂正手順を別に検討する。この候補の SealGraph 登録、CLI 契約候補を含む詳細設計の受入、正本設計・PERT・実装の改訂、コミット・push はそれぞれ別の後続段階である。
