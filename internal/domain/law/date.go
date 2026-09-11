package law

import "time"

// Date JSTの暦日。値はYYYY-MM-DD
type Date string

// JST 日本標準時。DBやAPIのタイムスタンプはこれで暦日に変換する
var JST = time.FixedZone("JST", 9*60*60)

const dateLayout = "2006-01-02"

// ParseDate 形式が違えばerrorを返す
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return "", err
	}
	return Date(t.Format(dateLayout)), nil
}

// Add dを日数分ずらす。月末や年末をまたぐ計算はtime.AddDateに委ねる。dはParseDateを通った値である前提で、パース失敗は無視する
func (d Date) Add(days int) Date {
	t, _ := time.ParseInLocation(dateLayout, string(d), JST)
	return Date(t.AddDate(0, 0, days).Format(dateLayout))
}

// Before dがoより前かどうかを返す。YYYY-MM-DDは文字列比較で暦順と一致する
func (d Date) Before(o Date) bool {
	return string(d) < string(o)
}

// String dをYYYY-MM-DDで返す
func (d Date) String() string {
	return string(d)
}

// DateOf tをJSTに変換して暦日にする
func DateOf(t time.Time) Date {
	return Date(t.In(JST).Format(dateLayout))
}
