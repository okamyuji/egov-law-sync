package law

import "regexp"

// idPattern IDに許す文字。APIとCSVから来たIDをそのままファイルパスとURLのパスに使うので、区切りになる文字を1つも通さない
var idPattern = regexp.MustCompile("^[0-9A-Za-z_]+$")

// ValidID sがIDとして使える文字だけでできているかを返す
func ValidID(s string) bool {
	return idPattern.MatchString(s)
}
