package tags

import (
	"strings"
)

// Get returns the value of tagName within tags.
func Get(tags []string, tagName string) string {
	for _, tag := range tags {
		parts := strings.Split(tag, ":")
		if len(parts) == 0 {
			continue
		}
		if parts[0] != tagName {
			continue
		}
		return parts[1]
	}
	return ""
}
