package obfuscate

import "testing"

func TestQuantizeIPSchemeIsAnchored(t *testing.T) {
	// The scheme must be a real prefix. A value like sftp://IP must not match
	// the ftp scheme as a substring, drop the leading s, and corrupt the value.
	for in, want := range map[string]string{
		"sftp://10.0.0.1":  "sftp://10.0.0.1",
		"xhttp://10.0.0.1": "xhttp://10.0.0.1",
	} {
		if got := quantizeIP(in); got != want {
			t.Fatalf("quantizeIP(%q) = %q, want %q", in, got, want)
		}
	}
}
