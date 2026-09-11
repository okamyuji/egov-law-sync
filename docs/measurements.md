# e-Gov法令API改訂追随ロジックツリー（2026-09-11実測）

計測対象: laws.e-gov.go.jp API v2、elaws.e-gov.go.jp API v1、bulkdownload。すべて本セッションでHTTP実測した値のみ記載。

## 前提事実（実測）

| 項目 | 観測値 |
|---|---|
| ETag / Last-Modified | 全エンドポイントで無し。`cache-control: no-store` |
| If-Modified-Since | 200が返る（304にならない） |
| 未知クエリパラメータ | 無視され200で全件（`updated_from`等は存在しない） |
| /lawsの絞り込み | law_id, law_num_*, law_title, law_type, asof, category_cd, promulgation_date_from/to, repeal_status, limit, offset, order（spec lawapi-v2.yaml） |
| 全法令数 | 9567 |
| /laws limit=5000 | 受理される。2リクエストで全件、gzip合計1.6MB、7.5s |
| gzip効果 | 1000件1.96MB → 137KB |
| law_data / law_file 1法令（労働基準法） | 約400KB、0.2s |
| 連続20リクエスト（law_revisions、12KB） | 全200、2.9s、レート制限ヘッダ無し |
| 現行本文データの更新件数 | 直近7日50件、30日373件、90日915件（v2の`updated`基準。再登録を含み、未施行改正を含まない） |
| 改正登録件数（v1 updatelawlists） | 09-08: 75件、09-09: 2件、09-10: 5件、09-11: 17件 |
| 更新時刻 | 日中随時。09-10は11:16 / 18:09 / 22:35 JST、09-11は08:58 JST（v2の`updated`） |
| bulkdownload sec3（日次更新zip） | 09-10: 363KB / 0.3s / 5法令、09-01: 3.6MB / 0.7s。CSVはUTF-8 BOM、列に 法令ID・施行日・未施行 |
| bulkdownload sec3の期間 | 文書上「過去3ヶ月」だが20260501（4ヶ月前）も200 / 7.6MB |
| bulkdownload sec1（全件zip） | 324MB、32.6s、Content-Length / ETag無し |
| v1 updatelawlists/{YYYYMMDD} | 200、0.26〜0.51s、38KB。LawId / AmendNo / EnforcementDateを含む |

## 差分ソースごとの検知対象（2026-09-10の1日分で照合）

| ソース | 件数 | 内容 |
|---|---|---|
| v1 updatelawlists | 5 | 新規登録された改正。未施行を含む（施行日2026-10-16, 2027-04-01, 2028-03-01） |
| bulkdownload sec3 | 5 | v1と完全一致（交差5） |
| v2 /laws `revision_info.updated` | 22 | 現行施行revisionのデータ更新時刻。v1との交差は2。残り20件は施行日が過去のrevisionの再登録（law_revisionsで全履歴のupdatedが09-10） |

v2一覧は現行施行revisionしか持たない。労働基準法の未施行改正（updated 07-23）は一覧に出ず、一覧のupdatedは07-17のまま。

## ロジックツリー

```
法令改訂への追随
├─ A.検知（変更信号を誰が出すか）
│  ├─ A0.サーバが通知する（RSS / Webhook）…… 未確認（トップページは800BのSPAシェル）。採用しない
│  ├─ A1.サーバが変更メタデータを出す・HTTP層（ETag / Last-Modified）…… 不可（無し、IMS→200）
│  ├─ A2.サーバが変更メタデータを出す・アプリ層
│  │  ├─ A2a.改正登録日リスト（v1 updatelawlists ＝ sec3 zip、09-10で同集合）
│  │  │       …… 未施行改正を含む。v1は38KB/0.3s、zipはXML同梱で0.3〜0.7s。v1の継続は未確認
│  │  └─ A2b.v2 /laws全件のupdated比較 …… 現行本文の再登録・訂正。1.6MB/7.5s。未施行改正は拾えない
│  └─ A3.クライアントが自分で比較する（本文ハッシュ）
│          …… 確定的だが重い。sec1 324MB/32.6s、個別なら9567×0.2s（law_data実測）≈ 32分（推定）
├─ B.取得（変わったものをどう取るか）
│  ├─ B1.個別law_data / law_file …… 400KB/0.2s、revision_id指定可
│  ├─ B2.日次zip（sec3）…… 更新分XMLのみ
│  ├─ B3.全件zip（sec1）…… 324MB、初期ロードと完全再同期
│  └─ B4.一覧JSON gzip …… メタデータのみ
└─ C.周期（どの頻度で回すか）
   ├─ 日次: A2a（前日分、日付が閉じてから）+ A2b → B1
   ├─ 施行日到来: law_revisionsのamendment_enforcement_dateを保持し切替
   └─ 週次〜月次: A3（sec1でハッシュ照合）
```

## 現実的な構成（実測値で選定）

1. 毎朝、前日分のsec3 zipを取る。新規改正と未施行改正をXML込みで1リクエストで得る。更新は22時台にも観測されたため当日分は翌朝に取る。
2. 毎日、v2 /lawsをlimit=5000 × 2で全件取得し、updatedを前回と比較する。sec3が拾わない再登録・訂正（09-10は20件）を検知し、差分だけlaw_dataで取る。gzip必須。
3. 週次〜月次でsec1全件zipと本文ハッシュを照合する。1〜2で取りこぼした分の保険。32.6s / 324MB。
4. 未施行改正はlaw_revisionsの施行日で管理する。一覧の自動切替は未確認。

## 未計測のため提示しないもの

反映側（DBスキーマ、差分レビュー）、実行基盤（cron / GitHub Actions）、push通知の有無、v1 APIの継続性、安全な並列度、sec3の正確な保持期間。

## 追加計測（2026-09-11、レビュー指摘に基づく）

| 項目 | 観測値 |
|---|---|
| sec1 zipのall_law_list.csv | 10702行（revision単位）、法令IDの種類は8997。列はsec3のCSVと同じ14列で、revision_id列とupdated列は無い。本文URLの末尾がrevision_idの日付部分 |
| sec1 zipのディレクトリ名 | 10702件すべてが`法令ID_YYYYMMDD_改正法令ID`の形式（revision_idと同じ） |
| /lawsの現行revisionのうちsec1 zipに無いもの | 623件。repeal_status別にRepeal 414、LossOfEffectiveness 105、Expire 38、None 66 |
| /lawsのrepeal_status分布 | None 9010、Repeal 414、LossOfEffectiveness 105、Expire 38 |
| law_file XMLとsec1 zip内XML | 労働基準法の現行revisionでsha256が一致（398337バイト、バイト単位で同一） |
| 未施行列の値 | 空文字（8998行）またはU+25CBの「○」（1704行） |
| /lawsのrevision_info.law_title | 存在する |
| /laws?asof=2027-04-01 | 総件数9567、現在とrevision_idが異なる法令は632件 |
| /laws?limit=1&offset=10000 | 200、`{"total_count":0,"count":0,"laws":[]}` |
| 更新の無い日（2026-09-05、土曜） | sec3は500（HTMLのエラーページ）、v1 updatelawlistsは404 |
| 2026-09-06（日曜） | sec3は200で1法令（11.5KB）、v1は200で1件 |
| 2026-09-07（月曜） | sec3は200、254KB |
| sec1 zipのヘッダ | `Content-Disposition: attachment; filename="all_xml.zip"`。Last-Modifiedは無く、構築日時は分からない |
| 利用規約 | デジタル庁サイトのコンテンツは公共データ利用規約（PDL1.0）で、出典の記載が必要。e-Gov法令検索サイト固有の規約ページはJavaScript描画のため未取得 |
| /law_file/xml（廃止・失効の法令） | Repeal、Expireの3件すべて200でXMLが返る（6KB、9KB、1.5KB、0.1秒） |
| /law_revisions（廃止法令） | 200。1件のrevisionでrepeal_statusとcurrent_revision_statusがRepeal |
| /laws?asof=2099-12-31 | 200、総件数9567。現在とrevision_idが異なる法令は842件 |
| /lawsのorder | 既定値はlaw_info.law_id。`order=law_id`は400（並び順が誤っています）。正しい形は`+law_info.law_id` |
| /lawsのtotal_count | 1ページ目は9567、countが0になる終端ページでは0 |
| v1 updatelawlistsの土日 | 08-15、08-16、08-22、08-23、08-29、08-30の6日すべて404 |
| /law_revisionsのamendment_law_num | 「令和八年法律第四十六号」のような法令番号の文字列 |
| sec1 zipの展開後サイズ | XML合計3618MB、最大17MB/件（1ファイル） |
| sec1 zip内のファイル配置 | `<revision_id>/<revision_id>.xml`。労働基準法の現行revisionで確認 |
| sec3の09-01分 | 3.6MB、79エントリ（CSV1つとディレクトリ39組） |
| HEAD /laws | 200 |
| /laws?limit=1 | 1784バイト、約1.0秒 |
| /laws?law_type=Act&limit=1000 | 総件数2165、1963603バイト、2.0秒 |
| /law_data（労働基準法、JSON） | 419817バイト、0.2秒 |
| /law_revisions（労働基準法） | 15件の履歴、12299バイト、0.1秒 |
| 仕様書lawapi-v2.yaml | 102875バイト |
| cache-controlの値 | `max-age=0, no-cache, no-store` |
| /lawsのrevision_infoの項目 | law_revision_id、law_type、law_title、law_title_kana、abbrev、category、updated、amendment_promulgate_date、amendment_enforcement_date、amendment_enforcement_comment、amendment_scheduled_enforcement_date、amendment_law_id、amendment_law_title、amendment_law_title_kana、amendment_law_num、amendment_type、repeal_status、repeal_date、remain_in_force、mission、current_revision_status |
| /lawsのlaw_infoの項目 | law_type、law_id、law_num、law_num_era、law_num_year、law_num_type、law_num_num、promulgation_date |
| sec3 zipのディレクトリ名 | sec1と同じ`法令ID_YYYYMMDD_改正法令ID`の形式（例: `329M50010000056_20260910_508M60000200043`） |
