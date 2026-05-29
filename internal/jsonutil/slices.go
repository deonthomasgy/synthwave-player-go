package jsonutil

// StringSlice returns a non-nil slice so JSON encodes as [] instead of null.
func StringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
