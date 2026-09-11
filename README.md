# egov-law-sync

e-Gov法令APIが公開する法令データの改訂を、GitHub Actionsだけで毎日追随する仕組みです。変更のあった法令を検知し、その日の改正登録と現行本文の状態をgit管理のmanifestに記録して、本文XMLはGitHub Releaseに添付します。

設計は[docs/design.md](docs/design.md)、計測の生記録は[docs/measurements.md](docs/measurements.md)にあります。この文書には、設計判断の根拠になった計測値と公開情報を載せています。

法令データの出典はデジタル庁の[e-Gov法令検索](https://laws.e-gov.go.jp/)です。デジタル庁のコンテンツは[公共データ利用規約（PDL1.0）](https://www.digital.go.jp/copyright-policy)の適用を受けます。Releaseに添付するXMLも同じ出典です。

## e-Gov法令APIの公開日

| 版 | 公開日 | 根拠 |
|---|---|---|
| Version 1 | 2017年（平成29年）6月26日 | e-Gov法令検索の公開と同日です。[総務省の公開案内](https://www.soumu.go.jp/main_content/000492195.pdf)に「公開時期：平成29年6月26日（月）午後〜」と「同時に、二次利用をしやすくするためのAPIや、バルク機能も提供」とあります。[v1仕様書1.4版](https://laws.e-gov.go.jp/file/houreiapi_shiyosyo.pdf)の変更履歴にも、1.0版が2017/6/26の新規作成と記されています |
| Version 2 | 2025年（令和7年）3月14日 | [デジタル庁のリリース案内](https://laws.e-gov.go.jp/file/%E6%B3%95%E4%BB%A4API%E3%83%90%E3%83%BC%E3%82%B8%E3%83%A7%E3%83%B32%E3%83%AA%E3%83%AA%E3%83%BC%E3%82%B9%E3%81%AE%E3%81%8A%E7%9F%A5%E3%82%89%E3%81%9B.pdf)に「2025年3月14日(金)にリリースしました」とあります |

同じリリース案内に「法令API Version1も当面の間ご利用可能です」とあり、v1の終了日は公表されていません。v1の`updatelawlists`は[v1ドキュメント](https://laws.e-gov.go.jp/docs/law-data-basic/8529371-law-api-v1/)に「指定可能な年月日は2020年11月24日以降」と明記されています。

## 実測で分かったこと

2026年9月11日に、API v2、API v1、bulkdownloadへ実際にHTTPリクエストを送って計測した値です。推測は含みません。

### HTTPの挙動

| 項目 | 観測値 |
|---|---|
| ETag / Last-Modified | 全エンドポイントで返りません。`cache-control: max-age=0, no-cache, no-store`です |
| If-Modified-Since | 200が返ります。304にはなりません |
| HEAD | 200が返ります |
| gzip | `Accept-Encoding: gzip`で圧縮されます。1000件の一覧が1.96MBから137KBになります |
| 未知のクエリパラメータ | 無視され、200で全件が返ります。`updated_from`のような差分パラメータは存在しません |
| 連続20リクエスト | すべて200で、合計2.9秒でした。レート制限を示すヘッダはありません。安全な並列度は未計測です |
| 仕様書 | `https://laws.e-gov.go.jp/api/2/swagger-ui/lawapi-v2.yaml`（102KB）にあります。エンドポイントは`/laws` `/law_revisions` `/law_data` `/attachment` `/keyword` `/law_file`の6つです |

### API v2の各エンドポイント

| エンドポイント | 観測値 |
|---|---|
| `/laws?limit=1` | 総件数9567件。約1.0秒 |
| `/laws?limit=5000` | 受理されます。offset=0と5000の2リクエストで全件が取れ、gzip合計1.6MB、7.5秒でした |
| `/laws?law_type=Act&limit=1000` | 法律は2165件。1.96MB、2.0秒 |
| `/laws`の絞り込み | law_id、law_num系、law_title、law_type、asof、category_cd、promulgation_date_from/to、repeal_status、limit、offset、orderです |
| `/law_data/{law_id}` | 労働基準法で約420KB（JSON）、0.2秒。XMLは約400KB |
| `/law_file/xml/{revision_id}` | 約398KB、0.2秒。`Content-Disposition: attachment`付きです |
| `/law_revisions/{law_id}` | 労働基準法で15件の履歴、12KB、0.1秒。各履歴に`updated`、`amendment_enforcement_date`、`current_revision_status`（CurrentEnforced、UnEnforced、PreviousEnforced、廃止法令ではRepeal）があります |

### 更新頻度と更新時刻

| 項目 | 観測値 |
|---|---|
| 現行本文データの更新件数（v2の`updated`基準） | 直近7日で50件、30日で373件、90日で915件。再登録を含み、未施行改正を含みません |
| 改正登録件数（v1 updatelawlists） | 09-08は75件、09-09は2件、09-10は5件、09-11は17件 |
| 更新時刻 | 日中随時です。09-10は11:16、18:09、22:35 JST、09-11は08:58 JSTに更新がありました |

### bulkdownload

| 項目 | 観測値 |
|---|---|
| sec3（日次更新zip） | 09-10分は363KB、0.3秒、5法令。09-01分は3.6MB、0.7秒、39法令。zipには`R080910.csv`（UTF-8 BOM付き）と法令ごとのディレクトリが入ります |
| sec3のCSV列 | 法令種別、法令番号、法令名、法令名読み、旧法令名、公布日、改正法令名、改正法令番号、改正法令公布日、施行日、施行日備考、法令ID、本文URL、未施行 |
| sec3の保持期間 | 仕様書には「過去3ヶ月」とありますが、4ヶ月前の20260501でも200で7.6MBが返りました |
| sec1（全件zip） | 324MB、32.6秒（約9.9MB/s）。Content-LengthとETagはありません |
| sec1の中身 | XMLが10702件、展開後3.6GB、最大17MB/件。`all_law_list.csv`が同梱されています。未施行のrevisionも含むため、一覧の9567件より多くなります |

### 全件zipと一覧の関係

| 項目 | 観測値 |
|---|---|
| all_law_list.csv | 10702行でrevision単位です。法令IDの種類は8997で、revision_id列とupdated列はありません |
| ディレクトリ名 | 10702件すべてが`法令ID_YYYYMMDD_改正法令ID`の形式で、revision_idと同じです。中央の8桁は施行日です |
| 一覧の現行revisionのうちzipに無いもの | 623件です。内訳はRepeal 414、LossOfEffectiveness 105、Expire 38、None 66で、廃止・失効の法令はzipに入りません |
| law_file XMLとzip内XML | 労働基準法の現行revisionでsha256が一致しました（バイト単位で同一） |
| 未施行列の値 | 空文字またはU+25CBの「○」です |
| offset=10000 | 200で空の配列が返ります。終端は空の配列で示されるので、countが0になるまで繰り返す方式で全件を取れます。総件数が10000件を超えたときの挙動は未検証です |
| asof=2027-04-01 | 現在とrevision_idが異なる法令は632件です。一斉施行日には数百件が同時に切り替わります |

### 更新の無い日の挙動

| 日付 | sec3 zip | v1 updatelawlists |
|---|---|---|
| 2026-09-05（土） | 500（HTMLのエラーページ） | 404 |
| 2026-09-06（日） | 200、1法令 | 200、1件 |
| 2026-09-07（月） | 200、254KB | 未計測 |

更新の無い日は「無い」ことが非200で表現されます。この仕組みはv1の404を「更新無し」と扱い、その日のsec3に更新があればv1側の異常として警告します。

### 差分ソースは別の事象を指す

2026年9月10日の1日分で照合しました。

| ソース | 件数 | 検知する事象 |
|---|---|---|
| v1 updatelawlists | 5 | その日に登録された改正です。未施行を含みます（施行日は2026-10-16、2027-04-01、2028-03-01） |
| bulkdownload sec3 | 5 | v1と完全に同じ集合でした |
| v2の`/laws`にある`revision_info.updated` | 22 | 現行施行revisionのデータ更新時刻です。v1との交差は2件で、残り20件は施行日が過去のrevisionの再登録でした |

v2の一覧は現行施行revisionしか持ちません。労働基準法には2026-07-23に登録された未施行改正がありますが、一覧の`updated`は2026-07-17のままです。未施行改正を追うにはsec3かv1、または`/law_revisions`が必要です。

### 採用した同期方式

```text
法令改訂への追随
├─ 検知
│  ├─ サーバ通知（RSS / Webhook）…… 見つからず。採用しない
│  ├─ HTTPキャッシュ検証（ETag等）…… 不可
│  ├─ 改正登録日リスト（v1 updatelawlists → /law_revisions）…… 日次。未施行含む
│  ├─ v2 /laws全件のupdated比較 …… 日次。再登録・訂正を検知
│  └─ 本文ハッシュ全比較（sec1 zip）…… 週次の保険
├─ 取得
│  ├─ 個別law_file XML …… 初回は全件、以降は差分の法令だけ
│  └─ 全件zip（sec1）…… 週次照合のみ
└─ 周期
   ├─ 日次: 前日分のv1リスト + v2一覧比較 → 差分XML取得
   └─ 週次: sec1全件とハッシュ照合
```

sec3 zipは、更新の無い日に500を返すことと、CSVにrevision_id列が無いことから、v1が終了したときの切り替え先として残しています。

### API v2の追加計測

| 項目 | 観測値 |
|---|---|
| `/law_file/xml`（廃止・失効の法令） | 3件すべて200でXMLが返ります |
| `/laws?asof=2099-12-31` | 現在とrevision_idが異なる法令は842件です。未施行改正を持つ法令の一覧として使えます |
| `/laws`のorder | 既定値は`law_info.law_id`です。`order=law_id`は400になります |
| `/laws`のtotal_count | 1ページ目は9567、終端ページでは0です |
| v1 updatelawlistsの土日 | 8月の土日6日分はすべて404でした |

### 実走の結果

GitHub Actionsでの実走の結果です。

| 実行 | 結果 |
|---|---|
| bootstrap（2026-09-11） | 一覧9568件、本文XMLを9568件すべて取得しました。合計1.19GB、44分（並列度1）です。Releaseに`laws-xml.zip`を添付しました |
| daily（2026-09-11、bootstrapと同日） | 対象範囲が空だったので何も取得せず、9.7秒で終わりました。想定外の変更は0件です |
| bootstrap再実行（2026-09-11、`v0.0.1`） | 一覧9568件、本文XMLとLLM向けテキストを9568件すべて作りました。合計1.19GB、49分（並列度1）です。`v0.0.1`のReleaseに`laws-xml.zip`（127MB）と`laws-text.zip`（189MB）を添付しました |

## できることとできないこと

この仕組みは「どの法令のどの版がいつから有効か」という事実と、その本文XMLを毎日最新に保ちます。法令を参照するエージェントやアプリケーションは、manifestのrevision_idで参照する版を固定できます。

法令の解釈は行いません。条文の意味の解釈、業務ロジックが現行法に準拠しているかの判定、改正が業務に与える影響の判断は対象外です。改正の検知もe-Gov側にデータが載った後に限られ、官報公布からe-Govへの反映までの時間差はこの仕組みでは埋められません。

税法（所得税法、法人税法、消費税法、相続税法、国税通則法、租税特別措置法と、その施行令・施行規則）はe-Gov法令APIの対象に含まれるので、本則の一覧、改正履歴、条文本文を取得できます。一方で、国税庁の法令解釈通達や質疑応答事例は法令ではないため取得できません。実務上の税務判断で参照する通達類は別途用意する必要があります。詳細は[設計書の「税法の扱い」](docs/design.md#税法の扱い)にあります。

## 使い方

```sh
make build                                                          # CGOなしでbin/egov-law-syncを作る
bin/egov-law-sync bootstrap                                         # 初回。/lawsとlaw_fileからmanifestを作る
bin/egov-law-sync daily --from 2026-09-09 --to 2026-09-10           # 範囲の差分を取り込む（省略時は前回適用済みの翌日からJSTの前日まで）
bin/egov-law-sync weekly                                            # sec1 zipとxml_index.csvを照合する
bin/egov-law-sync ingest bin/text                                   # MarkdownとJSONLの置き場からChunkSinkへ登録する（既定はno-op）
```

共通のオプションは`--release-tag`、`--xml-dir`（既定値`bin/xml`）、`--zip-path`（既定値`bin/laws-xml.zip`）、`--text-dir`（既定値`bin/text`。空なら変換しない）、`--text-zip-path`（既定値`bin/laws-text.zip`）です。`daily`はさらに`--from`、`--to`、`--force`を受け取ります。

Releaseのタグは`v0.0.1`のようなsemantic versionです。`MAJOR.MINOR`はリポジトリ直下の`VERSION`が持ち、変えるときは`VERSION`を書き換えるPRを出します。`PATCH`は実行のたびに既存のReleaseから自動で採番します。各Releaseには`laws-xml.zip`（取得したXMLとindex.csv）と`laws-text.zip`（同じ法令の`<revision_id>.md`、`<revision_id>.jsonl`、index.csv）が付きます。Markdownは先頭にlaw_id、revision_id、施行日、出典URLのfront matterを持ち、条を`####`の見出しにした本文が続きます。JSONLは1行が1条で、法令ID、revision_id、法令名、施行日、位置、条番号、見出し、本文、出典URLを持ちます。

終了コードは4つです。

| コード | 意味 |
|---|---|
| 0 | 正常に終わりました。警告があればruns/のJSONに入ります |
| 3 | 異常判定に該当しました。3つのCSVは書き換わりません |
| 2 | 使い方の誤りです。サブコマンドが無い、未知のサブコマンド、未知のフラグのときに返します |
| 1 | それ以外のエラーです。HTTPの失敗が再試行後も続く場合とファイルの読み書きの失敗が該当します |

環境変数で接続先と動作を変えられます。

| 環境変数 | 既定値 | 用途 |
|---|---|---|
| `EGOV_V2_BASE` | `https://laws.e-gov.go.jp/api/2` | API v2のベースURL |
| `EGOV_V1_BASE` | `https://elaws.e-gov.go.jp/api/1` | API v1のベースURL |
| `EGOV_BULK_BASE` | `https://laws.e-gov.go.jp` | bulkdownloadのベースURL |
| `EGOV_CONCURRENCY` | `1` | 本文取得の並列度 |
| `EGOV_MANIFEST_DIR` | `manifest` | 3つのCSVとruns/の置き場所 |

再試行後も失敗した取得は、原因をstderrに出します。出すのは1回の実行で先頭10件までで、残りは件数を1行にまとめたものです。

ローカルで実行した場合、`--release-tag`が空なので本文XMLの取得は行われず、laws.csv、revisions.csv、runs/だけが書き換わります。ローカル実行の結果はcommitしないでください。runs/をcommitすると次回のActionsの対象日がずれます。commitとReleaseの作成はGitHub Actionsのワークフローが行います。

GitHub Actionsでは`bootstrap.yml`を手動で1回起動し、その後は`daily.yml`が毎日07:00 JSTに、`weekly.yml`が毎週日曜08:00 JSTに動きます。正常時は3つのCSVを自動commitし、異常判定に該当した日はCSVを変えずにラベル`anomaly`のIssueを作ります。人が内容を確認したうえで適用する場合は、`daily.yml`を手動起動して`--force`を渡します。CIは`okamyuji/reusable-workflows@v1`のGo CIとsecurity-scanに加えて、`VERSION`の形式検査、`make doclint`、`make quality`を実行します。

### 開発時の検査

`make install-hooks`を一度実行すると、commitの前に`scripts/quality-gate.sh`が走ります。内容はgofmt、go vet、層の依存検査、staticcheck、golangci-lint、govulncheck、go build、shellテスト、doclint、go test（`-count=1 -shuffle=on -race`、カバレッジ80%以上）、CRAP値、mutation testing、E2E、gitleaksです。CIの`gates`ジョブも同じスクリプトを実行します。手で走らせるときは`make quality`です。staticcheck、golangci-lint、govulncheckは事前にインストールしておきます。

## 任意のvectorDBへ登録する

`laws-text.zip`を展開したディレクトリ（または実行後の`bin/text`）を`ingest`に渡すと、JSONLを1行ずつ読んで100件ずつ`ChunkSink`に渡します。既定のsinkは何も登録せず件数をstderrに出すだけなので、まずこれで読めることを確かめます。

```sh
unzip -q laws-text.zip -d text
bin/egov-law-sync ingest text
```

登録先を足すには、`internal/infrastructure/sink/<name>/sink.go`を1ファイル作り、`cmd/egov-law-sync/main.go`の`buildDeps`にある`Sink: &noop.Sink{}`を自分のsinkに差し替えます。interfaceは次のとおりです。

```go
type ChunkSink interface {
	Put(ctx context.Context, chunks []law.Chunk) error
}
```

`law.Chunk`のキーはlaw_id、revision_id、law_title、law_num、enforcement_date、path、article、article_title、text、source_urlです。embeddingの計算とAPIキーの扱いはsink側で行います。同じ条の再登録は`revision_id`と`article`を結合した値を主キーにして上書きします。このリポジトリは`go.mod`に依存を足さない方針なので、database/sqlのドライバやHTTPクライアントを使うsinkは別モジュールに置くか、forkで足します。

pgvectorの例です。

```sql
CREATE TABLE law_chunks (
  id text PRIMARY KEY,
  law_id text NOT NULL,
  revision_id text NOT NULL,
  law_title text NOT NULL,
  enforcement_date date,
  path text,
  article text,
  article_title text,
  body text NOT NULL,
  embedding vector(1536)
);
```

```go
func (s *Sink) Put(ctx context.Context, chunks []law.Chunk) error {
	for _, c := range chunks {
		vec, err := s.embed(ctx, c.Text)
		if err != nil {
			return err
		}
		id := string(c.RevisionID) + "|" + c.Article
		if _, err := s.db.ExecContext(ctx, `INSERT INTO law_chunks (id, law_id, revision_id, law_title, enforcement_date, path, article, article_title, body, embedding)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (id) DO UPDATE SET body = EXCLUDED.body, embedding = EXCLUDED.embedding, enforcement_date = EXCLUDED.enforcement_date`,
			id, c.LawID, c.RevisionID, c.LawTitle, c.EnforcementDate, c.Path, c.Article, c.ArticleTitle, c.Text, vec); err != nil {
			return err
		}
	}
	return nil
}
```

Qdrantの例では、`PUT /collections/laws/points`に`{"points":[{"id":"<revision_idとarticleから作ったUUID>","vector":[...],"payload":{"law_id":...,"revision_id":...,"article":...,"text":...}}]}`をまとめて送ります。payloadにはChunkのキーをそのまま入れます。接続先とAPIキーは環境変数から読みます。

## 所管省庁の通達・通知と組み合わせる

条文はこのアプリが揃えますが、実務で参照する所管省庁の通達、通知、ガイドライン、審査基準は法令ではないので、e-Gov法令APIには載りません。それらを同じ形式で揃えてLLMに条文と一緒に読ませる場合の設計と実装の概略です。実装はこのリポジトリの範囲外で、別のコマンドまたは別のリポジトリに置きます。

### 所管省庁の決め方

府省令は法令番号に発出元が入っています（例: 「令和六年厚生労働省令第十号」）。法律と政令は法令番号だけでは分からないので、同じ題名で始まる施行規則の発出元を所管とみなします（例: 「労働基準法施行規則」が厚生労働省令なら「労働基準法」は厚生労働省の所管）。どちらも`laws.csv`の題名と`laws-text.zip`のfront matterにある`law_num`から機械的に引けます。内閣府令や複数省庁の共管は候補を複数持たせ、人が確定します。

### 通達・通知の公開場所の例

| 所管 | 公開場所 | ページ名 |
|---|---|---|
| 国税庁 | `https://www.nta.go.jp/law/tsutatsu/menu.htm` | 法令解釈通達 |
| 国税庁 | `https://www.nta.go.jp/law/shitsugi/01.htm` | 質疑応答事例 |
| 厚生労働省 | `https://www.mhlw.go.jp/hourei/` | 法令等データベースサービス（通知の検索） |
| 金融庁 | `https://www.fsa.go.jp/common/law/` | 法令・指針等（監督指針、事務ガイドライン） |
| 公正取引委員会 | `https://www.jftc.go.jp/dk/guideline/` | 法令・ガイドライン等（独占禁止法） |
| 特許庁 | `https://www.jpo.go.jp/system/laws/rule/guideline/` | 基準・便覧・ガイドライン（審査基準） |

他の省庁も、法令の所管ページや行政手続法に基づく審査基準・処分基準の公開ページに同種の文書があります。省庁ごとに一覧ページのURLと本文ページの構造が違うので、所管ごとに「一覧URL、一覧から本文URLを取る規則、本文の分割規則」を設定ファイル（例: `circulars.yaml`）に持たせ、パイプラインは共通にします。

### 共通の流れ

1. 取得。[webgrab](https://github.com/okamyuji/webgrab)のようなHTML本文抽出ツールで一覧ページを定期的に取得し、前回との差分で新着と改正を検知します。本文ページをMarkdownにし、出典URLと取得日をfront matterに残します。省庁サイトには機械可読な更新一覧が無いので、取得件数の急変で止める保護をこのアプリの異常判定と同じ考え方で持ちます。ページ構造の変更で壊れる前提で監視します。
2. 分割。文書自身の項番号を単位にします（通達の「36-1」、監督指針の「II-1-2」、審査基準の章節など）。条文の条番号と一対一ではないので、条文とは別の規則です。
3. 対応付け。本文中の「労働基準法第三十六条第一項」のような参照を正規表現で拾い、`laws.csv`の題名で`law_id`に、条番号を`article`の値に変換して候補にします。確定は人またはLLMが行います。
4. 登録。`law.Chunk`と同じキーのJSONLにします。`path`に文書名、`article`に項番号、`law_id`と`article_title`に対応付けの候補を入れ、種別（条文か通達か）と発出元と発出日をpayloadに持たせて、このアプリの`ingest`と同じ`ChunkSink`で同じvectorDBに並べます。
5. 照合。質問に関連する条文チャンクと通達チャンクを検索して同時にLLMへ渡します。通達側の`law_id`と`article`で条文と結び、`revisions.csv`の施行日と通達の発出日を並べて「どの版の条文に対する解釈か」を示します。改正後の条文に古い通達しか無い場合はその旨を示します。解釈と適用の判断はLLMと人が行い、この仕組みは本文と対応関係を供給するところまでです。

税法では、所得税法などの本則と施行令・施行規則をこのアプリが揃え、国税庁の法令解釈通達と質疑応答事例を上の流れで並べる形になります。租税特別措置法は改正と部分施行が多いので、施行日と発出日の並記が特に役立ちます。

## 通知

初回スコープでは通知を実装していません。日次ジョブの差分確定直後に呼び出し位置があり、Slack Incoming Webhookで差分の法令ID、法令名、施行日を投稿する形を想定しています。詳細は[設計書の通知の節](docs/design.md#通知)にあります。

## 未確定の事項

- `law_file`のXMLとsec1 zip内のXMLの一致は労働基準法1件で確認しました。週次照合が、廃止・失効を除く現行revisionのうち取得済みのものについて確認し、不一致の件数を記録します。
- v1 APIの終了日は未公表です。更新の無い日の判定はv1の404に依存しています。
- 安全な並列度は未計測です。
- 施行日到来時に`/laws`の現行revisionが自動で切り替わるかは未確認です。

## ライセンス

MITライセンスです。全文は[LICENSE](LICENSE)にあります。
