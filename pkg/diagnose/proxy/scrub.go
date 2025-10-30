package proxy

import "net/url"

// RedactURL removes user:pass@ from URLs for display purposes.
// If parsing fails, returns the input unchanged.
func RedactURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if u.User != nil {
		u.User = url.User("****")
	}
	return u.String()
}
