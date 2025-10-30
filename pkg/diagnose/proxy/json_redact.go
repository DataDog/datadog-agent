package proxy

// Optional helpers for callers who want redacted JSON.
// The CLI prints redacted values already via RedactURL; JSON output
// currently returns the raw values (common Agent behavior).

func RedactEffectiveForJSON(e Effective) Effective {
	out := e
	out.HTTP.Value = RedactURL(e.HTTP.Value)
	out.HTTPS.Value = RedactURL(e.HTTPS.Value)
	// NO_PROXY is not redacted (doesn't carry credentials)
	return out
}
