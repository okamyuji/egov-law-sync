package sync

import "github.com/okamyuji/egov-law-sync/internal/domain/law"

type Thresholds struct {
	MaxUnexpected     int     // 200
	MaxTotalDropRatio float64 // 0.01
	MaxFetchFailures  int     // 20
}

// DefaultThresholds 設計文書の異常判定に定めた既定値を返す
func DefaultThresholds() Thresholds {
	return Thresholds{MaxUnexpected: 200, MaxTotalDropRatio: 0.01, MaxFetchFailures: 20}
}

// Anomaly 3つのCSVを適用しない理由。空なら正常
type Anomaly string

const (
	AnomalyTooManyChanges Anomaly = "too_many_unexpected_changes"
	AnomalyTotalDropped   Anomaly = "total_count_dropped"
)

// CheckAnomalies unexpectedとtotal_countの判定。prevTotalが0なら総件数の判定はしない。forceならどちらも判定しない
func CheckAnomalies(unexpected, prevTotal, curTotal int, t Thresholds, force bool) []Anomaly {
	if force {
		return nil
	}
	anomalies := make([]Anomaly, 0)
	if unexpected > t.MaxUnexpected {
		anomalies = append(anomalies, AnomalyTooManyChanges)
	}
	if prevTotal > 0 && float64(prevTotal-curTotal) > float64(prevTotal)*t.MaxTotalDropRatio {
		anomalies = append(anomalies, AnomalyTotalDropped)
	}
	return anomalies
}

// XMLTargets xml_index.csvに無いかupdatedが異なる法令
func XMLTargets(cur []law.Law, index map[law.RevisionID]law.XMLRecord) []law.Law {
	targets := make([]law.Law, 0)
	for _, c := range cur {
		rec, ok := index[c.RevisionID]
		if !ok || rec.Updated != c.Updated {
			targets = append(targets, c)
		}
	}
	return targets
}

// DateRange fromからtoまでの暦日。from > toなら空
func DateRange(from, to law.Date) []law.Date {
	dates := make([]law.Date, 0)
	if to.Before(from) {
		return dates
	}
	for d := from; ; d = d.Add(1) {
		dates = append(dates, d)
		if d == to {
			break
		}
	}
	return dates
}

// V1Range v1の一覧を取る範囲。fromの2日前からto
func V1Range(from, to law.Date) []law.Date {
	return DateRange(from.Add(-2), to)
}
