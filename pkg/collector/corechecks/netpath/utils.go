package netpath

// CopyStrings makes a copy of a list of strings
func CopyStrings(tags []string) []string {
	newTags := make([]string, len(tags))
	copy(newTags, tags)
	return newTags
}
