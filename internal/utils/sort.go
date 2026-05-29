package utils

import (
	"strconv"
	"strings"
	"unicode"
)

func NaturalCompare(a, b string) int {
	clean := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	pa := splitNum(clean(a))
	pb := splitNum(clean(b))
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na := pa[i]
		nb := pb[i]
		ea := len(na) > 0 && na[0] >= '0' && na[0] <= '9'
		eb := len(nb) > 0 && nb[0] >= '0' && nb[0] <= '9'
		if ea && eb {
			ia, _ := strconv.Atoi(na)
			ib, _ := strconv.Atoi(nb)
			if c := ia - ib; c != 0 {
				return c
			}
		} else {
			if c := strings.Compare(strings.ToLower(na), strings.ToLower(nb)); c != 0 {
				return c
			}
		}
	}
	return len(pa) - len(pb)
}

func splitNum(s string) []string {
	var parts []string
	var cur strings.Builder
	num := false
	for _, r := range s {
		d := unicode.IsDigit(r)
		if cur.Len() == 0 || d == num {
			cur.WriteRune(r)
			num = d
		} else {
			parts = append(parts, cur.String())
			cur.Reset()
			cur.WriteRune(r)
			num = d
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}
