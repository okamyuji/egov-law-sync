| 出典 | 目的 | 具体対象 | 役割 | 前後関係 | 初出定義 | 候補語 |
| --- | --- | --- | --- | --- | --- | --- |
| VERSION、ユーザー指摘 | 機能追加を番号に反映する | 現在の0.0を0.1へ更新し、次のReleaseをv0.1.1から採番する | 値 | PRマージ → 次回のRelease作成 | | VERSION |
| tools/next-version.sh | タグ形式を統一する | 既存Releaseの最大PATCHに1を足したvMAJOR.MINOR.PATCHを対象別Actionsにも使う | 手段 | VERSIONの読み取り → Release作成 | | next-version.sh |
| .github/workflows/law-filter.yml | 実行成果を識別する | 対象法令IDをRelease名に記し、タグは共通の番号で付ける | 記録 | 対象別同期 → Release作成 | | Release |
