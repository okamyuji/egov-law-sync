package law

import "testing"

func TestValidID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"実データのlaw_id", "322AC0000000049", true},
		{"実データのrevision_id", "322AC0000000049_20250601_507AC0000000015", true},
		{"英数字と下線", "A_1", true},
		{"1文字", "a", true},
		{"空文字", "", false},
		{"パス区切り", "a/b", false},
		{"親ディレクトリ", "../x", false},
		{"先頭のパス区切り", "/a", false},
		{"ハイフン", "a-b", false},
		{"ドット", "a.b", false},
		{"空白", "a b", false},
		{"クエリ記号", "a?b", false},
		{"全角", "あ", false},
		{"改行を含む", "a\nb", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ValidID(c.in); got != c.want {
				t.Fatalf("ValidID(%q)=%v want %v", c.in, got, c.want)
			}
		})
	}
}
