// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import (
	"testing"
)

func assertTokenSignature(t *testing.T, expected string, tok Token) {
	t.Helper()
	actual := tok.Signature()
	if actual != expected {
		t.Errorf("Signature of %v(%q) = %q, want %q", tok.Type, tok.Value, actual, expected)
	}
}

func assertMessageSignature(t *testing.T, message, expected string) {
	t.Helper()
	actual := MessageSignature(message)
	if actual != expected {
		t.Errorf("MessageSignature(%q)\n  got:  %q\n  want: %q", message, actual, expected)
	}
}

func TestSignatureSpecialCharacter(t *testing.T) {
	assertTokenSignature(t, "?", SpecialCharToken('?'))
	assertTokenSignature(t, ".", SpecialCharToken('.'))
	assertTokenSignature(t, ":", SpecialCharToken(':'))
}

func TestSignatureNumericValue(t *testing.T) {
	assertTokenSignature(t, "numericValue", NumericValueToken("454"))
	assertTokenSignature(t, "numericValue", NumericValueToken("0"))
	assertTokenSignature(t, "numericValue", NumericValueToken("-123"))
}

func TestSignatureEmailAddress(t *testing.T) {
	assertTokenSignature(t, "localPart@domain", EmailToken("dog", "datadoghq.com"))
}

func TestSignatureIPv4(t *testing.T) {
	assertTokenSignature(t, "ipv4", Token{Type: TypeIPv4Address, Value: "12.23.34.45"})
}

func TestSignatureIPv6(t *testing.T) {
	assertTokenSignature(t, "ipv6", Token{Type: TypeIPv6Address, Value: "2001:db8:85a3::8a2e:370:7334"})
}

func TestSignatureDate(t *testing.T) {
	assertTokenSignature(t, "yyyy-MM-dd HH:mm:ss",
		DateToken("yyyy-MM-dd HH:mm:ss", "2018-01-01 12:23:34"))
	assertTokenSignature(t, "yyyy-MM-dd",
		DateToken("yyyy-MM-dd", "2018-01-01"))
	assertTokenSignature(t, "yyyy-MM-ddTHH:mm:ss",
		DateToken("yyyy-MM-ddTHH:mm:ss", "2018-01-01T12:23:34"))
}

func TestSignatureLocalTime(t *testing.T) {
	assertTokenSignature(t, "local-time:HH:mm:ss",
		LocalTimeToken("HH:mm:ss", "12:56:45"))
}

func TestSignatureAuthority(t *testing.T) {
	hostReg := Token{Type: TypeWord, Value: "localhost"}
	assertTokenSignature(t, "regularHostName",
		AuthorityToken(&hostReg, 0, false, "", false))

	assertTokenSignature(t, "user@regularHostName",
		AuthorityToken(&hostReg, 0, false, "user", true))

	assertTokenSignature(t, "regularHostName:port",
		AuthorityToken(&hostReg, 22, true, "", false))

	hostIp := Token{Type: TypeIPv4Address, Value: "12.23.34.45"}
	assertTokenSignature(t, "user@ipv4:port",
		AuthorityToken(&hostIp, 22, true, "user", true))

	hostIp6 := Token{Type: TypeIPv6Address, Value: "2001:db8:85a3::8a2e:370:7334"}
	assertTokenSignature(t, "user@ipv6:port",
		AuthorityToken(&hostIp6, 22, true, "user", true))
}

func TestSignatureWord(t *testing.T) {
	assertTokenSignature(t, "word", WordToken("abc"))
	assertTokenSignature(t, "specialWord", WordToken("abc22"))
}

func TestSignatureAbsolutePath(t *testing.T) {
	path := PathToken([]string{"aa", "bb", "cc"})
	path.Type = TypeAbsolutePath
	assertTokenSignature(t, "AbsolutePath", path)
}

func TestSignaturePathWithQueryAndFragment(t *testing.T) {
	assertTokenSignature(t, "AbsolutePath",
		PathQueryFragmentToken([]string{"aa", "bb"}, nil, nil))

	emptyQ := ""
	assertTokenSignature(t, "AbsolutePath?",
		PathQueryFragmentToken([]string{"aa", "bb"}, &emptyQ, nil))

	q := "query"
	assertTokenSignature(t, "AbsolutePath?query",
		PathQueryFragmentToken([]string{"aa", "bb"}, &q, nil))

	f := "fragment"
	assertTokenSignature(t, "AbsolutePath#fragment",
		PathQueryFragmentToken([]string{"aa", "bb"}, nil, &f))

	assertTokenSignature(t, "AbsolutePath?query#fragment",
		PathQueryFragmentToken([]string{"aa", "bb"}, &q, &f))
}

func TestSignatureURI(t *testing.T) {
	host := Token{Type: TypeWord, Value: "localhost"}
	auth := AuthorityToken(&host, 0, false, "", false)
	path := PathToken([]string{"aa", "bb"})
	path.Type = TypeAbsolutePath

	assertTokenSignature(t, "http://regularHostNameAbsolutePath",
		URIToken("http", &auth, &path, nil, nil))

	q := "query"
	assertTokenSignature(t, "http://regularHostNameAbsolutePath?query",
		URIToken("http", &auth, &path, &q, nil))

	f := "fragment"
	assertTokenSignature(t, "http://regularHostNameAbsolutePath#fragment",
		URIToken("http", &auth, &path, nil, &f))

	assertTokenSignature(t, "http://regularHostNameAbsolutePath?query#fragment",
		URIToken("http", &auth, &path, &q, &f))
}

func TestSignatureHexDump(t *testing.T) {
	assertTokenSignature(t, "HexDump[dl:4|ascii:F]",
		HexDumpToken("0010: DB 8A 8E 01 20", 4, false))
	assertTokenSignature(t, "HexDump[dl:8|ascii:T]",
		HexDumpToken("00000000: 01 02 03 04 05 Ascii", 8, true))
}

func TestSignatureSeverity(t *testing.T) {
	assertTokenSignature(t, "Severity{value=*}", SeverityToken("INFO"))
	assertTokenSignature(t, "Severity{value=*}", SeverityToken("ERROR"))
}

func TestSignatureHttpMethod(t *testing.T) {
	assertTokenSignature(t, "HttpMethod{value=*}", HttpMethodToken("GET"))
	assertTokenSignature(t, "HttpMethod{value=*}", HttpMethodToken("POST"))
}

func TestSignatureHttpStatusCode(t *testing.T) {
	assertTokenSignature(t, "HttpStatusCode{value=*}", HttpStatusCodeToken("200"))
	assertTokenSignature(t, "HttpStatusCode{value=*}", HttpStatusCodeToken("404"))
}

func TestSignatureWhitespace(t *testing.T) {
	assertTokenSignature(t, " ", WhitespaceToken(1))
	assertTokenSignature(t, " ", WhitespaceToken(5))
}

func TestConcatenationToSpecialWord(t *testing.T) {
	assertMessageSignature(t, "a", "word")
	assertMessageSignature(t, "a.b", "specialWord")
	assertMessageSignature(t, "a:b", "specialWord")
	assertMessageSignature(t, "a:", "word:")
	assertMessageSignature(t, "a.", "word.")
	assertMessageSignature(t, ":a", ":word")
	assertMessageSignature(t, ".a", ".word")
	assertMessageSignature(t, "a..a", "word..word")
	assertMessageSignature(t, "a::a", "word::word")

	assertMessageSignature(t, "log_aws.s3.key", "specialWord")
	assertMessageSignature(t, "redis:v2732199", "specialWord")
}

func TestSignatureQAExample8(t *testing.T) {
	assertMessageSignature(t, "job missing monitoring_job_id kwarg", "word word word word")
	assertMessageSignature(t, "some jobs are late", "word word word word")
}

func TestSignatureQAExample18(t *testing.T) {
	msg := `Orphaned pod "ee57f5c4-f636-11eb-b9a5-02a1bd4bc06f" found, but volume paths are still present on disk : There were a total of 2 errors similar to this. Turn up verbosity to see them.`
	expected := `word word "specialWord" word, word word word word word word word word : word word word word word numericValue word word word word. word word word word word word.`
	assertMessageSignature(t, msg, expected)
}

func TestSignatureQAExample5(t *testing.T) {
	assertMessageSignature(t, "CUS-8007/lead-summary-fix", "specialWord/word")
}

func TestSignatureQAExample29(t *testing.T) {
	msg := `rid="Root=1-6117ecd1-2c50a4cf4b1c15085d2d7721" volatile_cache: cache_name=synthetics-pls, message="unable to get" exc="Timeout reading from socket"`
	sig := MessageSignature(msg)
	if sig == "" {
		t.Error("signature should not be empty")
	}
}

func TestSignatureConsistency(t *testing.T) {
	messages := []string{
		"[stats] total:889 rps:14.82",
		"[stats] total:890 rps:15.03",
		"new connection: 234",
		"new connection: 887",
	}

	sig1 := MessageSignature(messages[0])
	sig2 := MessageSignature(messages[1])
	if sig1 != sig2 {
		t.Errorf("similar messages should have same sig: %q vs %q", sig1, sig2)
	}
	sig3 := MessageSignature(messages[2])
	sig4 := MessageSignature(messages[3])
	if sig3 != sig4 {
		t.Errorf("similar messages should have same sig: %q vs %q", sig3, sig4)
	}
	if sig1 == sig3 {
		t.Error("different messages should have different sigs")
	}
}

func TestSignatureHexDumpMessage(t *testing.T) {
	assertMessageSignature(t, "0010: CF 8A 1A 01 00", "HexDump[dl:4|ascii:F]")
	assertMessageSignature(t, "0010: DB 8A 8E 01 00 .....", "HexDump[dl:4|ascii:T]")
	assertMessageSignature(t, "ll header: 00000000: 02 f3 82 0b 48 6f 02 62 2e 2c 2e bf 08 00",
		"word word: HexDump[dl:8|ascii:F]")
}
