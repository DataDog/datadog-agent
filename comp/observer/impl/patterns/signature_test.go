// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"testing"
)

func assertTokenSignature(t *testing.T, expected string, tok Token, version int) {
	t.Helper()
	actual := tok.Signature(version)
	if actual != expected {
		t.Errorf("Signature(v%d) of %v(%q) = %q, want %q", version, tok.Type, tok.Value, actual, expected)
	}
}

func assertMessageSignature(t *testing.T, message, expected string, version int) {
	t.Helper()
	actual := MessageSignature(message, version)
	if actual != expected {
		t.Errorf("MessageSignature(%q, v%d)\n  got:  %q\n  want: %q", message, version, actual, expected)
	}
}

func TestSignatureSpecialCharacter(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "?", SpecialCharToken('?'), v)
		assertTokenSignature(t, ".", SpecialCharToken('.'), v)
		assertTokenSignature(t, ":", SpecialCharToken(':'), v)
	}
}

func TestSignatureNumericValue(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "numericValue", NumericValueToken("454"), v)
		assertTokenSignature(t, "numericValue", NumericValueToken("0"), v)
		assertTokenSignature(t, "numericValue", NumericValueToken("-123"), v)
	}
}

func TestSignatureEmailAddress(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "localPart@domain", EmailToken("dog", "datadoghq.com"), v)
	}
}

func TestSignatureIPv4(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "ipv4", Token{Type: TypeIPv4Address, Value: "12.23.34.45"}, v)
	}
}

func TestSignatureIPv6(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "ipv6", Token{Type: TypeIPv6Address, Value: "2001:db8:85a3::8a2e:370:7334"}, v)
	}
}

func TestSignatureDate(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "yyyy-MM-dd HH:mm:ss",
			DateToken("yyyy-MM-dd HH:mm:ss", "2018-01-01 12:23:34"), v)
		assertTokenSignature(t, "yyyy-MM-dd",
			DateToken("yyyy-MM-dd", "2018-01-01"), v)
		assertTokenSignature(t, "yyyy-MM-ddTHH:mm:ss",
			DateToken("yyyy-MM-ddTHH:mm:ss", "2018-01-01T12:23:34"), v)
	}
}

func TestSignatureLocalTime(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "local-time:HH:mm:ss",
			LocalTimeToken("HH:mm:ss", "12:56:45"), v)
	}
}

func TestSignatureAuthority(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		hostReg := Token{Type: TypeWord, Value: "localhost"}
		assertTokenSignature(t, "regularHostName",
			AuthorityToken(&hostReg, 0, false, "", false), v)

		assertTokenSignature(t, "user@regularHostName",
			AuthorityToken(&hostReg, 0, false, "user", true), v)

		assertTokenSignature(t, "regularHostName:port",
			AuthorityToken(&hostReg, 22, true, "", false), v)

		hostIp := Token{Type: TypeIPv4Address, Value: "12.23.34.45"}
		assertTokenSignature(t, "user@ipv4:port",
			AuthorityToken(&hostIp, 22, true, "user", true), v)

		hostIp6 := Token{Type: TypeIPv6Address, Value: "2001:db8:85a3::8a2e:370:7334"}
		assertTokenSignature(t, "user@ipv6:port",
			AuthorityToken(&hostIp6, 22, true, "user", true), v)
	}
}

func TestSignatureWord(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "word", WordToken("abc"), v)
		if v < 5 {
			assertTokenSignature(t, "textWithDigits", WordToken("abc22"), v)
		} else {
			assertTokenSignature(t, "specialWord", WordToken("abc22"), v)
		}
	}
}

func TestSignatureAbsolutePath(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		path := PathToken([]string{"aa", "bb", "cc"})
		path.Type = TypeAbsolutePath
		if v > 6 {
			assertTokenSignature(t, "AbsolutePath", path, v)
		} else {
			assertTokenSignature(t, "/*/*/*", path, v)
		}
	}
}

func TestSignaturePathWithQueryAndFragment(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		var pathSig string
		if v > 6 {
			pathSig = "AbsolutePath"
		} else {
			pathSig = "/*/*"
		}

		assertTokenSignature(t, pathSig,
			PathQueryFragmentToken([]string{"aa", "bb"}, nil, nil), v)

		emptyQ := ""
		assertTokenSignature(t, pathSig+"?",
			PathQueryFragmentToken([]string{"aa", "bb"}, &emptyQ, nil), v)

		q := "query"
		assertTokenSignature(t, pathSig+"?query",
			PathQueryFragmentToken([]string{"aa", "bb"}, &q, nil), v)

		f := "fragment"
		assertTokenSignature(t, pathSig+"#fragment",
			PathQueryFragmentToken([]string{"aa", "bb"}, nil, &f), v)

		assertTokenSignature(t, pathSig+"?query#fragment",
			PathQueryFragmentToken([]string{"aa", "bb"}, &q, &f), v)
	}
}

func TestSignatureURI(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		host := Token{Type: TypeWord, Value: "localhost"}
		auth := AuthorityToken(&host, 0, false, "", false)
		path := PathToken([]string{"aa", "bb"})
		path.Type = TypeAbsolutePath

		var expected string
		if v > 6 {
			expected = "http://regularHostNameAbsolutePath"
		} else {
			expected = "http://regularHostName/*/*"
		}
		assertTokenSignature(t, expected,
			URIToken("http", &auth, &path, nil, nil), v)

		q := "query"
		assertTokenSignature(t, expected+"?query",
			URIToken("http", &auth, &path, &q, nil), v)

		f := "fragment"
		assertTokenSignature(t, expected+"#fragment",
			URIToken("http", &auth, &path, nil, &f), v)

		assertTokenSignature(t, expected+"?query#fragment",
			URIToken("http", &auth, &path, &q, &f), v)
	}
}

func TestSignatureHexDump(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "HexDump[dl:4|ascii:F]",
			HexDumpToken("0010: DB 8A 8E 01 20", 4, false), v)
		assertTokenSignature(t, "HexDump[dl:8|ascii:T]",
			HexDumpToken("00000000: 01 02 03 04 05 Ascii", 8, true), v)
	}
}

func TestSignatureSeverity(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "Severity{value=*}", SeverityToken("INFO"), v)
		assertTokenSignature(t, "Severity{value=*}", SeverityToken("ERROR"), v)
	}
}

func TestSignatureHttpMethod(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "HttpMethod{value=*}", HttpMethodToken("GET"), v)
		assertTokenSignature(t, "HttpMethod{value=*}", HttpMethodToken("POST"), v)
	}
}

func TestSignatureHttpStatusCode(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, "HttpStatusCode{value=*}", HttpStatusCodeToken("200"), v)
		assertTokenSignature(t, "HttpStatusCode{value=*}", HttpStatusCodeToken("404"), v)
	}
}

func TestSignatureWhitespace(t *testing.T) {
	for v := 3; v <= LatestSignatureVersion; v++ {
		assertTokenSignature(t, " ", WhitespaceToken(1), v)
		assertTokenSignature(t, " ", WhitespaceToken(5), v)
	}
}

// --- Message-level concatenation to specialWord (v5+) ---

func TestConcatenationToSpecialWord(t *testing.T) {
	for v := 5; v <= LatestSignatureVersion; v++ {
		assertMessageSignature(t, "a", "word", v)
		assertMessageSignature(t, "a.b", "specialWord", v)
		assertMessageSignature(t, "a:b", "specialWord", v)
		assertMessageSignature(t, "a:", "word:", v)
		assertMessageSignature(t, "a.", "word.", v)
		assertMessageSignature(t, ":a", ":word", v)
		assertMessageSignature(t, ".a", ".word", v)
		assertMessageSignature(t, "a..a", "word..word", v)
		assertMessageSignature(t, "a::a", "word::word", v)

		assertMessageSignature(t, "log_aws.s3.key", "specialWord", v)
		assertMessageSignature(t, "redis:v2732199", "specialWord", v)
	}
}

func TestNoConcatenationBeforeV5(t *testing.T) {
	assertMessageSignature(t, "a.b", "word.word", 3)
	assertMessageSignature(t, "a.b", "word.word", 4)
	assertMessageSignature(t, "a:b", "word:word", 3)
}

func TestSignatureQAExample8(t *testing.T) {
	for _, v := range []int{3, 4, 5} {
		assertMessageSignature(t, "job missing monitoring_job_id kwarg",
			"word word word word", v)
		assertMessageSignature(t, "some jobs are late",
			"word word word word", v)
	}
}

func TestSignatureQAExample18(t *testing.T) {
	msg := `Orphaned pod "ee57f5c4-f636-11eb-b9a5-02a1bd4bc06f" found, but volume paths are still present on disk : There were a total of 2 errors similar to this. Turn up verbosity to see them.`
	expectedV5 := `word word "specialWord" word, word word word word word word word word : word word word word word numericValue word word word word. word word word word word word.`
	assertMessageSignature(t, msg, expectedV5, 5)
}

func TestSignatureQAExample5(t *testing.T) {
	// CUS-8007/lead-summary-fix
	for _, v := range []int{3, 4} {
		assertMessageSignature(t, "CUS-8007/lead-summary-fix",
			"textWithDigits/word", v)
	}
	for _, v := range []int{5, 6} {
		assertMessageSignature(t, "CUS-8007/lead-summary-fix",
			"specialWord/word", v)
	}
}

func TestSignatureQAExample29(t *testing.T) {
	msg := `rid="Root=1-6117ecd1-2c50a4cf4b1c15085d2d7721" volatile_cache: cache_name=synthetics-pls, message="unable to get" exc="Timeout reading from socket"`
	sig5 := MessageSignature(msg, 5)
	if sig5 == "" {
		t.Error("signature should not be empty")
	}
}

func TestSignatureVersionDifferences(t *testing.T) {
	// Word with digits: textWithDigits in v3/v4, specialWord in v5+
	assertMessageSignature(t, "abc123", "textWithDigits", 3)
	assertMessageSignature(t, "abc123", "textWithDigits", 4)
	assertMessageSignature(t, "abc123", "specialWord", 5)
}

func TestSignatureQAExample32(t *testing.T) {
	msg := `rid="Root=1-611df52d-369599bb6070a7d00172174f" Client side HTTPError: Number expected (For input string: "OK")`

	sig3 := MessageSignature(msg, 3)
	sig5 := MessageSignature(msg, 5)

	if sig3 == "" || sig5 == "" {
		t.Error("signatures should not be empty")
	}
	if sig3 == sig5 {
		// v3 has textWithDigits, v5 has specialWord - they should differ
		t.Error("v3 and v5 should have different signatures for this message")
	}
}

func TestSignatureConsistency(t *testing.T) {
	messages := []string{
		"[stats] total:889 rps:14.82",
		"[stats] total:890 rps:15.03",
		"new connection: 234",
		"new connection: 887",
	}

	for _, v := range []int{3, 4, 5, 6, 7, 8} {
		sig1 := MessageSignature(messages[0], v)
		sig2 := MessageSignature(messages[1], v)
		if sig1 != sig2 {
			t.Errorf("v%d: similar messages should have same sig: %q vs %q", v, sig1, sig2)
		}
		sig3 := MessageSignature(messages[2], v)
		sig4 := MessageSignature(messages[3], v)
		if sig3 != sig4 {
			t.Errorf("v%d: similar messages should have same sig: %q vs %q", v, sig3, sig4)
		}
		if sig1 == sig3 {
			t.Errorf("v%d: different messages should have different sigs", v)
		}
	}
}

func TestSignatureHexDumpV8(t *testing.T) {
	assertMessageSignature(t, "0010: CF 8A 1A 01 00", "HexDump[dl:4|ascii:F]", 8)
}

func TestSignatureQAExample52(t *testing.T) {
	assertMessageSignature(t, "0010: DB 8A 8E 01 00 .....", "HexDump[dl:4|ascii:T]", 8)
}

func TestSignatureQAExample53(t *testing.T) {
	assertMessageSignature(t, "ll header: 00000000: 02 f3 82 0b 48 6f 02 62 2e 2c 2e bf 08 00",
		"word word: HexDump[dl:8|ascii:F]", 8)
}
