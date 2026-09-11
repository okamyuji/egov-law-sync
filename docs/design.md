# egov-law-sync設計書

e-Gov法令APIが公開する法令データの改訂を、人手を介さずに追随するための仕組みです。数値と挙動の根拠はすべて[計測記録](measurements.md)にあります。計測は2026年9月11日に行いました。版はv6で、レビュー6回目までの指摘を反映して凍結しています。

## 目的

法令の改正が登録されたこと、および現行本文のデータが更新されたことを毎日検知し、変更のあった法令だけを取得して記録します。記録はgitの履歴として残り、差分は人が読める形で確認できます。エージェントやアプリケーションが「今どの法令のどの版を参照すべきか」を機械的に判定できる状態を保つことがゴールです。

## 要望との対応と限界

この仕組みの出発点は、「業務ロジックが現行法に準拠しているかを確認するエージェントに、常に正しい現行法を参照させたい。法改正のたびに人手で追随するのは難しい」という要望です。要望の各要素とこの設計の対応は次のとおりです。

| 要望の要素 | この設計での対応 | 限界 |
|---|---|---|
| 常に現行法を参照させる | laws.csvが各法令の現行施行revision_idを持ち、xml_index.csvとReleaseに本文XMLがあります。参照側はrevision_idで版を固定できます | 参照側がlaws.csvを読む実装は参照側の責務です。この仕組みはデータを整えるところまでです |
| 改正に追随する | 日次で改正登録と現行本文の更新を検知し、差分をgitの履歴として残します。未施行改正はrevisions.csvで施行日つきで先行把握できます。bootstrap時点で既に登録済みの未施行改正も取り込みます | e-Gov側のデータ反映には官報公布からの入力作業の時間差があります。この仕組みはe-Govに載った後にしか検知できません |
| 法令の解釈 | 行いません | 条文の意味の解釈、業務ロジックへの適用可否の判定、改正が業務に与える影響の判断は、この仕組みの対象外です。この仕組みが提供するのは「どの法令のどの版がいつから有効か」という事実と、その本文だけです |
| 変更を知らせる | gitの差分、異常と警告のIssue、拡張点としてのSlack通知があります | 初回スコープでは差分の通知を実装しません |

## 決定表

| 決定 | 検討した選択肢 | 選定 | 理由 |
|---|---|---|---|
| 変更の検知手段 | HTTPキャッシュ検証、更新日リスト、一覧のupdated比較、本文ハッシュ全比較 | 更新日リストと一覧のupdated比較を日次で併用し、本文ハッシュ全比較を週次で行う | ETagとLast-Modifiedは無く、If-Modified-Sinceは200を返します。更新日リストと一覧のupdatedは別の事象を検知するので両方が必要です（09-10の照合で5件と22件、交差2件） |
| 更新日リストの取得元 | v1 updatelawlists、bulkdownload sec3 zip | v1 updatelawlistsを主、v1が404の日だけsec3で照合 | 09-10の照合で両者は同じ集合でした。sec3はCSVにrevision_id列が無く法令単位で、更新の無い日に500を返します。v1は更新の無い日に404を返し（土日6日分で確認）、法令IDだけを渡せば/law_revisionsが改正の全項目を返します。v1が404でsec3に更新がある日は、v1側の異常として警告します |
| 改正の詳細の取得元 | sec3のCSVとディレクトリ名、/law_revisions | /law_revisions | revision_id、施行日、updated、改正法令番号、状態がAPIの項目として揃います。IDの文字位置からの導出やCSVの対応付けが不要です |
| 一覧の取得方法 | limit=100で96ページ、limit=5000で固定2ページ、limit=5000でcountが0になるまで繰り返す | limit=5000でcountが0になるまで繰り返し、収集件数を1ページ目のtotal_countと照合する。gzip。orderは既定値 | 実測でoffset=10000は200で空の配列を返します。固定2ページでは総件数が10000件を超えたときに静かに欠落します。途中のページが空で返る障害は件数の照合で検知します。orderの既定値はlaw_info.law_idで、誤った値は400です |
| 本文の取得元 | law_data JSON、law_file XML、sec1 zip | law_file XML | 廃止・失効の法令でも200でXMLが返ります（3件で確認）。sec1 zipには廃止・失効の法令が入りません。労働基準法ではlaw_fileとsec1のXMLがバイト一致しました |
| 正本の置き場所 | git内の3つのCSV、CSVのみ、外部ストレージ、SQLite | 3つのCSVをgit、XMLはGitHub Release | 全件XMLは展開後3.6GBで、gitには置けません。Releaseの1ファイル上限は2GBです |
| 異常時の扱い | 常に自動commit、常にPR、異常時はPR、異常時はIssue | 正常時は自動commit。異常時は3つのCSVを変更せず、runs/だけをcommitしてIssueを作る。警告はCSVを適用したうえでIssueを作る | CSVの差分を持つPRは、放置中の再実行、mainとの競合、マージ後のRelease欠落を生みます。Issueなら正本に触れず、人が判断してから手動起動の`--force`で適用できます |
| 変更件数の閾値 | 固定、対象日数に比例 | 固定200件 | 対象日数に比例させると、異常を放置した日数だけ閾値が増え、人の確認なしに自動適用される経路ができます。長期の欠落からの復帰は人が`--force`で行います |
| 実行基盤 | GitHub Actions cron、常駐サーバ、サーバレス | GitHub Actions cron | 公開リポジトリのActionsは実行時間に課金がありません。必要な権限はGITHUB_TOKENだけで、追加のSecretは初回スコープにありません |
| 実装言語 | Go、Python、Rust、TypeScript | Go1.27、実行時の外部依存なし | zip、CSV、JSON、HTTP、sha256がすべて標準ライブラリにあります |
| アプリケーション構成 | 機能ごとのパッケージ、DDDの4層 | DDDの4層。層間はinterfaceのみ | ユースケースをportのフェイクで単体テストでき、HTTPクライアントとCSVの実装を差し替えられます |
| 本文取得の並列度 | 逐次、固定並列 | 逐次（設定値で変更可、初期値1） | 安全な並列度は未計測です。連続20リクエストは全て200でしたがレート制限ヘッダはありません |
| 日次の対象日 | 実行日当日、前日、前回適用済みの翌日から前日まで | 前回適用済みの対象日の翌日から前日まで（JST）。v1の一覧はその2日前から取り直す | 更新は22時台にも観測されました。Actionsのスケジュールは遅延や欠落があるので、欠落日を次回に取り込みます。前日分のv1が翌朝の時点で確定しているかは未確認なので、2日分を重ねて取り直します |
| 想定内の変更 | 施行日が対象期間内、revisions.csvに登録済みのrevisionへの切替 | 実行開始時に読み込んだrevisions.csvに登録済みのrevisionへの切替 | 施行日到来による切替がe-Gov側でいつ反映されるかは未確認です。登録済みかどうかは反映日に依存しません。同じ実行の中で引き直した/law_revisionsを基準にすると切替が常に想定内になるので、基準は実行開始時の内容に固定します |
| 通知 | Slack Incoming Webhook、GitHub Issue、無し | 異常と警告はGitHub Issue。差分の通知はSlack Incoming Webhookを拡張点として設ける | 変更検知と記録を先に動かし、実測を得ることを優先します |
| 出典表示 | 無し、READMEに明記 | READMEとRelease本文に出典を明記 | デジタル庁のコンテンツは公共データ利用規約（PDL1.0）の適用で、利用時に出典の記載が求められます。法令検索サイト固有の規約は未確認です |

## 構成

リポジトリは`okamyuji/egov-law-sync`です。公開リポジトリとして作成します。

アプリケーションはDDDの4層で構成し、層と層はinterfaceを通してだけ接続します。依存の向きは外側から内側への一方向で、domainは他のどの層にも依存しません。

```text
cmd/egov-law-sync/              interface層。CLIの引数解釈、依存の組み立て、終了コードの決定
internal/domain/law/            domain層。Law、Revision、XMLRecordのエンティティと値オブジェクト
internal/domain/sync/           domain層。差分の4分類、想定内の判定、異常と警告の規則（純粋関数）
internal/application/           application層。Bootstrap、Daily、Weeklyのユースケースと、それらが要求するport（interface）
internal/infrastructure/egov/   infrastructure層。API v2、v1、bulkdownloadのHTTPクライアント（portの実装）
internal/infrastructure/csv/    infrastructure層。laws.csv、revisions.csv、xml_index.csv、runs/の読み書き（portの実装）
internal/infrastructure/zip/    infrastructure層。sec1 zipの展開とRelease用zipの作成（portの実装）
e2e/                            主要導線のE2Eテスト。ビルドしたバイナリを偽のAPIサーバに向けて実行する
manifest/                       正本。3つのCSVとruns/
.github/workflows/              ci.yml, bootstrap.yml, daily.yml, weekly.yml
docs/                           設計書と計測記録
tools/doclint.sh                設計文書とGoコメントの機械検査
tools/crap.sh                   CRAP値の計算
Makefile                        help、build（CGO_ENABLED=0）、test、crap、mutate、e2e、lint、doclint、clean
```

`bin/`、`docs/_quality/`（品質基準とセルフレビュー記録）、`docs/superpowers/`（設計のspecと実装計画）は中間成果物で、`.gitignore`によりgit管理しません。

### 層の責務とport

| 層 | 責務 | 依存してよい層 |
|---|---|---|
| domain | エンティティ、値オブジェクト、差分と異常判定の純粋な規則 | なし |
| application | ユースケースの手順。外部との入出力はportのinterfaceを通す | domain |
| infrastructure | portの実装。HTTP、CSV、zip、ファイルシステム | domain、application（portの型のみ） |
| interface | CLI。infrastructureのコンストラクタを呼んでportの実装を組み立て、ユースケースのinterfaceに渡し、結果を終了コードに変換する | application、infrastructure（コンストラクタのみ） |

application層が定義するportは次のとおりです。infrastructure層がこれを実装し、interface層の`main`が組み立てます。ユースケース自身も`Bootstrapper`、`DailySyncer`、`WeeklyChecker`のinterfaceとして公開します。

| port | 役割 | 実装 |
|---|---|---|
| LawCatalog | /lawsの全件取得（asof指定を含む）。1ページ目のtotal_countも返す | egov |
| RevisionSource | /law_revisionsの取得 | egov |
| XMLSource | /law_fileのXML取得。取得したXMLは`--xml-dir`のディレクトリに`<revision_id>.xml`として書く | egov |
| UpdateList | v1 updatelawlistsの取得。404は「無し」として返し、それ以外の失敗はエラーとして返す | egov |
| DailyArchive | sec3 zipの取得。500は「無し」として返し、200ならディレクトリ名の一覧を返す | egov、zip |
| BulkArchive | sec1 zipの取得と展開 | egov、zip |
| ManifestRepository | 3つのCSVとruns/の読み書き | csv |
| ReleaseBundler | XMLの一時保存先ディレクトリからRelease添付用zipを作る | zip |
| Clock | JSTの現在日時 | 標準ライブラリを包む小さな実装 |

日次の手順8にある差分通知の呼び出し位置は、application層の関数1つです。初回スコープでは何もしません。portにはしません。

Actionsのワークフローは`okamyuji/reusable-workflows@v1`のGo CIとsecurity-scanを呼び、それに加えて`make doclint`、`make e2e`、`make crap`、`make mutate`を実行するジョブを`ci.yml`に持ちます。

## 実装規約

- 関数とメソッドのドキュメントコメントは「名前 説明」の形で書きます。「名前 は 説明」のように助詞を挟みません。例は`// Classify 前回と今回のlaws.csvを比較して変更を4種類に分ける`です。
- コメントと文書の日本語では、数値や英単語の前後に半角ブランクを入れません。「limit=5000で」「Go 1.27」のような版数の並記も詰めて「Go1.27」と書きます。doclintが表の中とGoのコメント行も検査します。
- コメントには、コードから読み取れない理由や制約だけを書きます。変更履歴やタスクIDは書きません。例外は日次の手順8の通知の呼び出し位置で、依頼者の指示によりSlack Incoming Webhook案を書きます。
- application層がinfrastructure層の型を参照するコードは、レビューで差し戻します。interface層の`main`だけがinfrastructureのコンストラクタを呼びます。

## データ

正本は`manifest/`配下の3つのCSVと`runs/`です。3つのCSVはそれぞれ独立に正しい状態を持ちます。laws.csvは/lawsの写し、revisions.csvは/law_revisionsの写し、xml_index.csvは取得済みXMLの記録です。ファイル間で「同じ取得回の値でなければならない」という制約は置きません。参照側は、laws.csvのrevision_idをxml_index.csvで引き、無ければAPIから取ります。

### manifest/laws.csv

/lawsの全件の写しです。law_id順に並べて毎回全体を書き直すので、gitの差分がそのまま変更一覧になります。/lawsから消えた法令の行は削除します。消えた法令のrevision_idはxml_index.csvとrevisions.csvに残ります。

| 列 | 出典（/laws） |
|---|---|
| law_id | law_info.law_id |
| law_type | law_info.law_type |
| law_title | revision_info.law_title |
| revision_id | revision_info.law_revision_id |
| updated | revision_info.updated |
| enforcement_date | revision_info.amendment_enforcement_date |
| repeal_status | revision_info.repeal_status |

### manifest/revisions.csv

/law_revisionsの写しです。この仕組みが/law_revisionsを引いた法令について、その法令の全revisionを持ちます。revision_idを主キーとし、同じrevision_idはAPIの最新の値で置き換えます。first_seenだけは置き換え時も保持します。

| 列 | 出典（/law_revisions） |
|---|---|
| revision_id | law_revision_id |
| law_id | 親のlaw_info.law_id |
| law_title | law_title |
| enforcement_date | amendment_enforcement_date |
| promulgate_date | amendment_promulgate_date |
| amendment_law_num | amendment_law_num |
| status | current_revision_status（CurrentEnforced、UnEnforced、PreviousEnforced、Repealなど） |
| updated | updated |
| first_seen | この仕組みが最初に記録した日（JST、YYYY-MM-DD） |

未施行かどうかはstatusがUnEnforcedかどうかで判定します。

### manifest/xml_index.csv

取得済みの本文XMLの記録です。revision_idを主キーとします。

| 列 | 内容 |
|---|---|
| revision_id | XMLのrevision |
| updated | この行を最後に書いた時点でlaws.csvが持っていたupdated。日次の取得時も週次の差し替え時もlaws.csvの値を書く |
| sha256 | XMLのハッシュ |
| xml_bytes | XMLのバイト数 |
| release_tag | XMLを添付したReleaseのタグ。CLIの`--release-tag`で受け取る |

日次は、laws.csvの各行について「revision_idがxml_index.csvに無い」または「updatedが異なる」なら本文を取得します。取得に失敗した行はxml_index.csvを変えないので、翌日に再び取得対象になります。`--release-tag`が空のとき（ローカル実行）は本文を取得せず、xml_index.csvも変えません。

### manifest/runs/

`runs/daily/`、`runs/weekly/`、`runs/bootstrap/`に`YYYYMMDDTHHMMSSZ.json`を置きます。ファイル名は実行時刻（UTC）です。各ファイルは実行日（JST、date_jst）、秒数、異常と警告の内容を持ちます。bootstrapと日次のファイルは/lawsの1ページ目のtotal_countも持ちます。日次のファイルはさらに、対象日の範囲（from、to）、適用したかどうか（applied）、4分類の件数、想定内以外の件数、変更の一覧（law_id、種類、旧revision_id、新revision_id）、本文取得の成功と失敗の件数、バイト数、/law_revisionsの取得に失敗した法令ID（pending_law_ids）を持ちます。対象範囲が空の実行では、fromとtoに前回のtoをそのまま書きます。

次回の対象日の開始は、mainにある日次ファイルのうちappliedがtrueのもののtoの最大値の翌日です。pending_law_idsの引き継ぎ元は、直近の日次ファイルとbootstrapファイルのうち実行時刻が新しい方です。日次ファイルが1つも無い場合は、runs/bootstrap/の最新ファイルのdate_jstです。どちらも無い場合は、bootstrapが未実行として終了コード1で終わります。過去日を手動で再処理してもtoの最大値は下がりません。総件数の判定は、appliedがtrueの最新の日次ファイル、それが無ければ最新のbootstrapファイルのtotal_countと比較します。

### GitHub Release

| タグ | 作成タイミング | 添付 |
|---|---|---|
| `bootstrap-YYYYMMDDTHHMMSSZ` | bootstrap実行時 | 全法令の現行revisionのXMLとindex.csvをまとめたzip |
| `sync-YYYYMMDDTHHMMSSZ` | 日次または週次で本文を1件でも取得した実行 | 取得したXMLとindex.csvをまとめたzip |

タグは実行時刻なので重複せず、上書きは行いません。タグはワークフローが決めてCLIに`--release-tag`で渡し、CLIはxml_index.csvのrelease_tagにその値を書きます。CLIは取得したXMLを`--xml-dir`のディレクトリに`<revision_id>.xml`として書き、ReleaseBundlerがそのディレクトリからzipを作ります。bootstrapでは約3.2GBがディスクに置かれますが、Actionsのディスク約14GBに収まります。

zipの契約は次のとおりです。

- アセット名は`laws-xml.zip`です。
- zip内は平坦で、`<revision_id>.xml`と`index.csv`だけを含みます。
- index.csvの列はrevision_id、sha256、xml_bytesです。
- zip後のサイズは、sec1 zipの324MBと展開後3.6GBの比から、bootstrapで約290MBの見込みです。

Release本文には出典（e-Gov法令検索）、件数、対象日の範囲だけを書き、revision_idの一覧は書きません。任意のrevisionのXMLは、xml_index.csvのrelease_tagが示すReleaseの`laws-xml.zip`から`<revision_id>.xml`を取り出せます。

## 処理の流れ

### 共通の前提

- 3つのワークフローは同じ`concurrency`グループ`manifest`を使い、同時に1つしか動きません。
- HTTPは1リクエストのタイムアウト60秒（sec1 zipは600秒）、失敗時は2秒、4秒の間隔で最大3回再試行します。
- CLIの終了コードは、正常0、異常3、それ以外のエラー1です。終了コード0でも警告があればruns/にその内容が入ります。ワークフローは0と3以外の終了コード（1、panicの2、強制終了）をすべて同じ失敗経路として扱います。
- CLIのオプションは`--from`、`--to`、`--force`、`--release-tag`、`--xml-dir`です。`--from`と`--to`はv1の一覧を取る対象日にだけ影響し、/lawsの比較と本文取得は常に実行時点の/lawsに対して行います。本文取得の並列度は環境変数`EGOV_CONCURRENCY`（初期値1）で指定します。
- ワークフローの手順は「CLI実行、Releaseの作成と添付、commit、push」の順です。Releaseの作成に失敗すればcommitせずにジョブを失敗させ、次回の実行が本文を取り直します。pushが競合で失敗した場合もジョブを失敗させ、次回の実行が同じ範囲を再処理します。CLIが0で終わった後にジョブが失敗した場合も、`if: failure()`の手順でラベル`anomaly`のIssueを作ります。Releaseだけが残っても正本は参照しないので害はありません。
- ワークフローは終了コードを変数に取ります。3のときはruns/だけをcommitしてラベル`anomaly`のIssueを作ります。0でも3でもないときは、ラベル`anomaly`のIssueだけを作ります。0で警告があるときは、commitのあとにラベル`warning`のIssueを作ります。2つのラベルは、ワークフローの先頭で`gh label create --force`により毎回作ります（既にあれば何もしません）。同じラベルのIssueが開いていれば、新しいIssueは作らずそのIssueにコメントを追加するので、異常と警告は互いに埋もれません。
- 手動起動（workflow_dispatch）では`--from`、`--to`、`--force`を渡せます。`--force`は「想定内以外の変更件数」と「総件数の減少」の判定を無視して適用します。一覧の取得失敗と件数の不一致は`--force`があっても異常のままです。

### bootstrap（初回。再実行可能）

1. /lawsを全件取得し、収集件数を1ページ目のtotal_countと照合します。不一致なら、runs/bootstrap/だけを書いて終了コード3です。
2. /laws?asof=2099-12-31を全件取得し、現在とrevision_idが異なる法令（実測で842件）を未施行改正を持つ法令とみなします。それらの法令の/law_revisionsを取得します。取得に失敗した法令IDはpending_law_idsとしてruns/に書き、次の日次が引き直します。
3. 既存のrevisions.csvがあれば読み込み、手順2の結果をrevision_idで置き換えながらマージします。first_seenは既存行の値を保持し、新規行は実行日です。
4. 手順1で取得した一覧の全行について、law_file XMLを逐次取得します。既存のxml_index.csvがあれば読み込み、取得した行をrevision_idで置き換えながらマージします（過去のrevisionの行とrelease_tagは残ります）。release_tagには`bootstrap.yml`が渡す`--release-tag`の値を書きます。`--release-tag`が空なら本文取得を飛ばし、xml_index.csvを変えません。9567件×0.2秒で約32分の見込みです。失敗した行はxml_index.csvに書かず、件数をruns/に記録します。
5. laws.csv、revisions.csv、xml_index.csv、runs/bootstrap/を一時ファイルへ書き、renameで置き換えます。runs/bootstrap/にはtotal_countを含めます。取得したXMLが1件以上あれば、ReleaseBundlerで`bin/laws-xml.zip`を作ります。
6. ワークフローが`bootstrap-<実行時刻>`のReleaseに取得したXMLのzipを添付し、正本をcommitしてpushします。

bootstrapは`bootstrap.yml`を手動起動して実行します。再実行すると本文は全件取り直しになりますが、xml_index.csvとrevisions.csvはどちらもマージなので過去の行は残ります。HTTPの失敗が3回再試行後も続く場合は終了コード1で終わり、何も書きません。

### daily（毎日07:00 JST）

対象日の範囲は「前回適用済みのtoの翌日」から「JSTの前日」までです。範囲が空（前日まで適用済み）のときは手順1を飛ばします。手順1から5は取得と判定だけを行い、CSVへの書き込みは手順7でまとめて行います。

1. 対象範囲の2日前からtoまでの各日について、v1 updatelawlistsを取得します。200なら法令IDの集合に加えます。404の日は、その日が実行日の90日以内であればsec3 zipを取得し、200でディレクトリが1つ以上あれば警告「v1に無い更新がsec3にある」を記録し、500でも200でもなければ警告「sec3が取得できない」を記録し、そのディレクトリ名を`_`で区切った先頭要素を法令IDとして集合に加えます。
2. /lawsを全件取得し、収集件数を1ページ目のtotal_countと照合します。1ページでも3回再試行後に失敗した場合、または件数が不一致の場合は、一覧をもう1回だけ取り直し、それでも不一致なら3つのCSVを書かず、runs/daily/だけを書いて終了コード3で終わります。手順1でv1が404以外の失敗を3回再試行後も返した場合も同じです。
3. 前回のlaws.csvと比較し、変更ありの法令を4種類に分けます。
   - 新規（law_idが無かった）
   - 切替（revision_idが異なる）
   - 再登録（revision_idが同じでupdatedが異なる）
   - 消失（/lawsに無い）
4. 手順1の法令ID、手順3の新規と切替と再登録の法令ID、前回のruns/のpending_law_idsについて、/law_revisionsを取得します。revisions.csvに書く内容は、既存行をrevision_idで置き換え（first_seenは保持）、無い行を追加したものです。取得に失敗した法令IDはpending_law_idsとしてruns/に書き、警告にします。
5. 切替のうち、新しいrevision_idが実行開始時に読み込んだrevisions.csvにあるものを想定内とします。想定内以外の件数は、新規、消失、再登録、想定内でない切替の合計です。この件数が200件を超えたら異常です。1ページ目のtotal_countが前回の値より1%超少ない場合も異常です。異常のときは3つのCSVを書かず、runs/だけを書いて終了コード3で終わります。手順4で取得した法令IDはrevisions.csvに残らないので、すべてpending_law_idsに書きます。Issue本文には変更の一覧の先頭200行を載せます。
6. `--release-tag`が空なら本文取得を飛ばします。それ以外は、新しいlaws.csvの各行について、xml_index.csvに無いか、updatedが異なるなら、law_file XMLを逐次取得し、sha256とxml_bytesを計算します。失敗した行は記録だけして次へ進みます。失敗が20件を超えたら警告です。
7. laws.csv、revisions.csv、xml_index.csv、runs/daily/を一時ファイルへ書き、renameで置き換えます。appliedはtrueです。xml_index.csvのrelease_tagには`--release-tag`の値を書きます。手順6で取得したXMLが1件以上あれば、ReleaseBundlerで`bin/laws-xml.zip`を作ります。
8. 差分通知の呼び出し位置です。初回スコープでは何もしません。
9. 終了コード0で終わります。ワークフローは、手順6で1件でも取得していれば`sync-<実行時刻>`のReleaseにXMLのzipを添付し、正本をcommitしてpushします。警告があればIssueを作ります。

Actionsのワークスペースは実行ごとに破棄されるので、途中で止まった実行の中間状態はリポジトリに残りません。

### weekly（毎週日曜08:00 JST）

1. sec1 zipをディスクに一時保存し、実行後に破棄します。
2. laws.csvのうちrepeal_statusがNoneの行について、zip内の`<revision_id>/<revision_id>.xml`のsha256を計算し、xml_index.csvのsha256と比較します。xml_index.csvに無い行は飛ばします。
3. `--release-tag`が空なら手順3を飛ばします。不一致の行については、law_file XMLを取得し直します。取得したXMLのsha256がxml_index.csvと同じなら「zipが古い」として件数だけ記録します。異なるならxml_index.csvのupdated（laws.csvの現在の値）、sha256、xml_bytes、release_tagを更新し、XMLをReleaseに含めます。
4. zipに無いrevision（実測で廃止・失効以外に66件）とzipにあってlaws.csvに無いrevisionは件数だけ記録します。
5. xml_index.csvとruns/weekly/を書きます。手順3で差し替えたXMLが1件以上あれば、ReleaseBundlerで`bin/laws-xml.zip`を作ります。終了コード0で終わります。ワークフローは、手順3で取得したXMLがあれば`sync-<実行時刻>`のReleaseを作り、正本をcommitしてpushします。HTTPの失敗が続く場合は終了コード1です。

週次には異常判定がありません。週次はxml_index.csvとReleaseを直接直します。

### 日付とcron

ActionsのcronはUTCで書きます。

| ジョブ | JST | cron（UTC） |
|---|---|---|
| daily | 毎日07:00 | `0 22 * * *` |
| weekly | 日曜08:00 | `0 23 * * 6` |

対象日と実行日（date_jst）はジョブ内でJSTで計算します。runs/のファイル名だけがUTCです。

### ワークフローの権限

3つのワークフローは`permissions: contents: write, issues: write`を宣言します。PRは作らないので、リポジトリ設定の変更は不要です。

## 異常判定

異常は3つのCSVを適用せずIssueを作ります。警告は適用したうえでIssueを作ります。閾値は計測記録の値から決めています。

| 種類 | 判定 | 閾値 | 根拠 |
|---|---|---|---|
| 異常 | 想定内以外の変更件数が多すぎる | 200件超（固定） | 一覧のupdated差分の実測は09-10で22件、30日で373件（平均12件/日）でした。登録済みrevisionへの切替は想定内なので、一斉施行日にも登録済みの分は発火しません。16日を超える欠落からの復帰は再登録だけで200件を超えうるので（1日平均12.4件）、人が`--force`で適用します |
| 異常 | 一覧の総件数が減った | 1ページ目のtotal_countが前回のappliedな実行より1%超少ない | 全件は9567件です。初回は前回が無いので判定しません |
| 異常 | 一覧が取得できない、または件数が合わない | 1ページでも3回再試行後に失敗、または収集件数が1ページ目のtotal_countと不一致 | 部分的な一覧で比較すると最大5000件が消失と誤判定されます |
| 異常 | v1が取得できない | 404以外の失敗が3回再試行後も続く | 404は更新無しですが、それ以外は取りこぼしになります。2日分の重ね取りでは2日続く障害を吸収できません |
| 警告 | v1に無い更新がsec3にある | v1が404の日にsec3が200でディレクトリが1つ以上 | 09-10の照合で両者は同じ集合でした。v1の終了や障害をこの警告で知ります |
| 警告 | /law_revisionsが取得できない法令がある | 3回再試行後の失敗が1件以上 | 失敗した法令IDは次回に引き直すので欠落は残りません |
| 警告 | 本文取得の失敗が多い | 3回再試行後の失敗が20件超 | 個別取得は0.2秒で安定していました。少数の失敗は翌日に再取得します |

## 通知

異常と警告はGitHub Issueで知らせます。差分の通知は初回スコープでは実装せず、日次ジョブの手順8に呼び出し位置を設けます。この位置のコード内コメントにSlack Incoming Webhook案を書くのは、コメント規約（非自明なWHYのみ）の例外で、依頼者の明示的な指示によるものです。

Slack Incoming Webhookを使う場合の形は次のとおりです。

- Secret`SLACK_WEBHOOK_URL`にWebhook URLを置きます。
- 差分があった日に、law_id、法令名、施行日、未施行の有無を1メッセージで投稿します。
- 異常や警告のIssueを作った日は、IssueのURLを投稿します。

月次チェックなどの業務ロジックが参照している法令IDの一覧と突き合わせれば、見直しが必要な法令だけを通知する使い方もできます。

## テスト

| 種類 | 対象 | 方法 |
|---|---|---|
| 単体 | domain層の規則（差分の4分類、想定内の判定、異常と警告の境界値）、application層のユースケース（portをフェイクに差し替える）、infrastructure層の変換（v1 XMLの解析、/law_revisionsの変換、CSVの読み書き、zipの展開） | 純粋関数はテーブル駆動、ユースケースはportのフェイク、HTTPクライアントはhttptestで固定応答を返します |
| E2E | 主要導線。bootstrapの後にdailyを2回（更新あり、更新なし）、weeklyを1回、異常（閾値超過）を1回 | `make build`で作ったバイナリを、httptestで立てた偽のAPIサーバとbulkdownloadに向けて実行し、CSVの内容と終了コードを検証します |
| 統合 | 実APIへの取得 | `EGOV_LIVE=1`を設定したときだけ実行します |
| 実走 | Actions上のbootstrapとdaily | bootstrapは再実行可能なので繰り返し検証できます。runs/の値で確認します |

単体テストのカバレッジは80%以上を維持します。分岐の網羅は主要な規則に限り、細かな分岐まで厳密には求めません。品質のゲートは次の4つで、`ci.yml`はすべてを実行します。

| ゲート | 対象 | 閾値 | 道具 |
|---|---|---|---|
| `make test` | `cmd/`と`e2e/`を除く全パッケージ | 合計カバレッジ80%以上 | go test -cover |
| `make crap` | `make test`と同じパッケージの全関数とメソッド | CRAP値15以下。CRAP値はcc²×(1−cov)³+ccで、ccは循環的複雑度、covは関数のカバレッジ | gocyclo、go tool cover -func、tools/crap.sh |
| `make mutate` | `internal/domain/`配下のみ | 生存mutant0 | gremlins |
| `make e2e` | 主要導線5本 | 全件成功 | go test ./e2e/... |

CRAP値はカバレッジ80%のもとでは循環的複雑度13以下とほぼ同じ意味になり（cc=13で14.4、cc=14で15.6）、複雑な関数を分割させます。mutation testingをdomain層に限るのは、差分の分類や閾値や日付範囲の純粋関数が境界値の誤りを単体テストで見逃しやすい場所であり、infrastructure層の変異はE2Eで導線ごと検証する方が費用対効果が高いためです。gocycloとgremlinsは`go run`で版を固定して呼び、go.modには入れません。

## 未確定

- sec1 zipの再構築頻度は未計測です。週次はzipが古い場合を「zipが古い」として件数に記録するだけで、正本を壊しません。
- repeal_statusがNoneなのにsec1 zipに無い66件の理由は未確認です。週次で件数を記録し、増減を見ます。
- 安全な並列度は未計測です。逐次取得で日次の所要時間を計測し、必要になってから並列化を検討します。
- 施行日到来時に/lawsの現行revisionがいつ切り替わるかは未確認です。切替がいつ起きても、登録済みrevisionへの切替は想定内です。
- v1 updatelawlistsの終了日は未公表です。終了はsec3との照合の警告で検知します。その時点で取得元をsec3に切り替えます。切り替え先の挙動は計測記録にあります。
- 22時台の更新がその日のv1の一覧に入るのか翌日分に入るのか、また前日分のv1が翌朝07:00 JSTの時点で確定しているかは未確認です。日次はv1を対象範囲の2日前から取り直し、切替と新規の法令は/law_revisionsを引き直すので、どちらでも翌日以降に取り込まれます。
- 総件数が10000件を超えたときのoffsetの挙動は、総件数が9567件の時点では検証できません。countが0になるまで繰り返す方式と件数の照合で、欠落があれば異常として検知します。
- e-Gov法令検索サイト固有の利用規約は、ページがJavaScript描画のため未確認です。
