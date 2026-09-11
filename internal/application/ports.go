package application

import (
	"context"
	"time"

	"github.com/okamyuji/egov-law-sync/internal/domain/law"
	"github.com/okamyuji/egov-law-sync/internal/domain/sync"
)

// LawCatalog /lawsの全件。asofが空なら現時点。totalは1ページ目のtotal_count
type LawCatalog interface {
	ListAll(ctx context.Context, asof string) (laws []law.Law, total int, err error)
}

// RevisionSource /law_revisionsの取得。404はfound=false、errなし。それ以外の失敗はerr
type RevisionSource interface {
	Revisions(ctx context.Context, id law.LawID) (revs []law.Revision, found bool, err error)
}

// XMLSource dirに<revision_id>.xmlとして保存し、sha256とバイト数を返す
type XMLSource interface {
	FetchXML(ctx context.Context, id law.RevisionID, dir string) (sha256 string, n int64, err error)
}

// UpdateList v1 updatelawlists。404はfound=false、errなし。それ以外の失敗はerr
type UpdateList interface {
	UpdatedLawIDs(ctx context.Context, d law.Date) (ids []law.LawID, found bool, err error)
}

// DailyArchive sec3 zip。500はfound=false。200ならディレクトリ名の一覧
type DailyArchive interface {
	RevisionDirs(ctx context.Context, d law.Date) (dirs []string, found bool, err error)
}

// Archive 展開したsec1 zipへの読み取り。SHA256は<rev>/<rev>.xmlのハッシュ。無ければok=false
type Archive interface {
	SHA256(id law.RevisionID) (sum string, ok bool, err error)
	RevisionIDs() []law.RevisionID
	Close() error
}

// BulkArchive sec1 zipの取得
type BulkArchive interface {
	FetchAll(ctx context.Context) (Archive, error)
}

// RunRecord runs/の1ファイル
type RunRecord struct {
	Kind          string         `json:"kind"`
	StartedAt     string         `json:"started_at"`
	DateJST       string         `json:"date_jst"`
	From          string         `json:"from,omitempty"`
	To            string         `json:"to,omitempty"`
	Applied       bool           `json:"applied"`
	TotalCount    int            `json:"total_count"`
	Counts        map[string]int `json:"counts"`
	Changes       []sync.Change  `json:"changes,omitempty"`
	PendingLawIDs []law.LawID    `json:"pending_law_ids,omitempty"`
	Anomalies     []string       `json:"anomalies,omitempty"`
	Warnings      []string       `json:"warnings,omitempty"`
	Bytes         int64          `json:"bytes"`
	Seconds       float64        `json:"seconds"`
}

// ManifestRepository 正本の3つのCSVとruns/への読み書き
type ManifestRepository interface {
	LoadLaws() ([]law.Law, error)
	SaveLaws([]law.Law) error
	LoadRevisions() (map[law.RevisionID]law.Revision, error)
	SaveRevisions(map[law.RevisionID]law.Revision) error
	LoadXMLIndex() (map[law.RevisionID]law.XMLRecord, error)
	SaveXMLIndex(map[law.RevisionID]law.XMLRecord) error
	SaveRun(r RunRecord) error
	LastAppliedDaily() (RunRecord, bool, error)
	LastBootstrap() (RunRecord, bool, error)
	LastDaily() (RunRecord, bool, error)
}

// TextRenderer xmlDirの<revision_id>.xmlを読み、textDirへ<revision_id>.mdと<revision_id>.jsonlを書く。両方書けたときだけ成功
type TextRenderer interface {
	Render(xmlDir, textDir string, meta law.Law) (law.TextRecord, error)
}

// ChunkSource dir直下の*.jsonlをファイル名順に読み、1行ずつfnに渡す。fnのerrorで止まる
type ChunkSource interface {
	Each(dir string, fn func(law.Chunk) error) error
}

// ReleaseBundler Release添付のzipを作る。Bundleは<rev>.xml、BundleTextは<rev>.mdと<rev>.jsonl。どちらもindex.csvを含む
type ReleaseBundler interface {
	Bundle(xmlDir, outPath string, index []law.XMLRecord) error
	BundleText(textDir, outPath string, index []law.TextRecord) error
}

// Clock 実行時刻。テストのために固定できるようにする
type Clock interface {
	Now() time.Time
}

// Result ユースケースの結果。ExitCodeは0、3
type Result struct {
	ExitCode int
	Record   RunRecord
}
