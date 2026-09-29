# agmemx v7 CLI 契約受入記録

- 判定: Accepted（所有者、2026-09-29）
- 対象: [cli-contract-v7-candidate.md](cli-contract-v7-candidate.md) の再レビュー済み全文
- 対象 SHA-256: `b64378004eadba4c3befb11e528a1169a1fe387bf6c09138f11b76ec94bef3ff`
- 上流: [Accepted v7 要件](requirements-crud-associative-v7-acceptance.md) が固定する要件本文 SHA-256 `71f3d9b43008ade1365a3bd0b067129cd12b27bf4b9d785c247659aa06e364bf`、[Accepted v7 基本設計](architecture-v7-acceptance.md) が固定する基本設計本文 SHA-256 `c164bfbcd983e1d69551a01aa3c31af07b3cf51865df4b988930bdf029455247`
- 同時に審査した詳細設計: [v7 詳細設計受入記録](detailed-design-v7-acceptance.md) が固定する `detailed-design-v7-candidate.md` の SHA-256 `44799ff0dc143312ca12a7325b8f565d936c211442645dace41665d6a1f9321c`
- 経路: 詳細設計と CLI 契約を同じ境界で再レビューし、信念の自然文理由の保存先と空範囲での明示的な埋め込み検索の2件を修正した版について、受入阻害なしと報告した。所有者が両版を「受け入れます」と明示した。

受け入れた CLI 契約は上記の正確な版である。対象文書中の `Candidate` 表示は受入前の状態を記したもので、この記録は対象文書のバイト列を変更しない。受入範囲には、引数または JSON の入力経路、記録・関係・所属・検索の操作目録、text・JSON の応答、help・schema・補完・man の一致、検索と worker の埋め込みプロファイル選択、および Link の候補提示と明示 ID 指定の規則を含む。

この受入は v7 要件・基本設計・詳細設計を変更しない。旧 CLI・旧データとの互換、実装・テストの適合、正本 CLI 文書・PERT の改訂、SealGraph の登録、コミット、push、利用者への提供を示さない。
