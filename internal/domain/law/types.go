package law

type LawID string
type RevisionID string

// Law /lawsの1件の写し。laws.csvの1行に対応する
type Law struct {
	ID              LawID
	Type            string
	Title           string
	Abbrev          string
	RevisionID      RevisionID
	Updated         string // /lawsのrevision_info.updatedをそのまま
	EnforcementDate string // YYYY-MM-DD
	RepealStatus    string // None、Repeal、LossOfEffectiveness、Expire
}

// Revision /law_revisionsの1件の写し。revisions.csvの1行に対応する
type Revision struct {
	ID              RevisionID
	LawID           LawID
	Title           string
	EnforcementDate string
	PromulgateDate  string
	AmendmentLawNum string
	Status          string // CurrentEnforced、UnEnforced、PreviousEnforced、Repealなど
	Updated         string
	FirstSeen       string // YYYY-MM-DD、置き換え時も保持
}

// XMLRecord 取得済みXMLの記録。xml_index.csvの1行に対応する
type XMLRecord struct {
	RevisionID RevisionID
	Updated    string
	SHA256     string
	Bytes      int64
	ReleaseTag string
}
