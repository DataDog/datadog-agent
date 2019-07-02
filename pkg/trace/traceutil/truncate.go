package traceutil

// TruncateUTF8 truncates the given string to make sure it uses less than limit bytes.
// If the last character is an utf8 character that would be splitten, it removes it
// entirely to make sure the resulting string is not broken.
func TruncateUTF8(s string, limit int) string {
	var lastValidIndex int
	for i := range s {
		if i > limit {
			return s[:lastValidIndex]
		}
		lastValidIndex = i
	}
	return s
}
