package util

import "regexp"

const (
	apiKeyReplacement = "api_key=*************************$1"
)

var apiKeyRegExp = regexp.MustCompile("api_key=*\\w+(\\w{5})")

// SanitizeURL sanitizes credentials from a message containing a URL, and returns
// a string that can be logged safely.
// For now, it obfuscates the API key.
func SanitizeURL(message string) string {
	return apiKeyRegExp.ReplaceAllString(message, apiKeyReplacement)
}
