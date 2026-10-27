package fileserver

import (
	"cmp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// compareNames orders file names the way file explorers do: ignoring case,
// and comparing runs of digits by their numeric value, so that "file2" comes
// before "file10". Names that only differ in case are ordered byte-wise to
// keep the order deterministic.
func compareNames(a, b string) int {
	if c := naturalCompare(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		if isDigit(a[0]) && isDigit(b[0]) {
			da, db := digitPrefix(a), digitPrefix(b)
			// Without leading zeros, a longer number is a larger one.
			na, nb := strings.TrimLeft(da, "0"), strings.TrimLeft(db, "0")
			if c := cmp.Compare(len(na), len(nb)); c != 0 {
				return c
			}
			if c := strings.Compare(na, nb); c != 0 {
				return c
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		ra, sizeA := utf8.DecodeRuneInString(a)
		rb, sizeB := utf8.DecodeRuneInString(b)
		if c := cmp.Compare(unicode.ToLower(ra), unicode.ToLower(rb)); c != 0 {
			return c
		}
		a, b = a[sizeA:], b[sizeB:]
	}
	return cmp.Compare(len(a), len(b))
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

func digitPrefix(s string) string {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i]
}
