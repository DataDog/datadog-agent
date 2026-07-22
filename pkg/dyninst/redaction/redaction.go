// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package redaction

import (
	"strings"
	"unicode"
)

// defaultIdentifiers is the shared cross-language set of sensitive keywords,
// stored in normalized form (lowercase, separators already stripped). It is
// kept in sync with the Java, .NET, and Python tracers, which all derive from
// the Sentry SDK scrubber list.
var defaultIdentifiers = []string{
	"2fa",
	"accesstoken",
	"aiohttpsession",
	"apikey",
	"apisecret",
	"apisignature",
	"appkey",
	"applicationkey",
	"auth",
	"authorization",
	"authtoken",
	"ccnumber",
	"certificatepin",
	"cipher",
	"clientid",
	"clientsecret",
	"connectionstring",
	"connectsid",
	"cookie",
	"credentials",
	"creditcard",
	"csrf",
	"csrftoken",
	"cvv",
	"databaseurl",
	"dburl",
	"encryptionkey",
	"encryptionkeyid",
	"geolocation",
	"gpgkey",
	"ipaddress",
	"jti",
	"jwt",
	"licensekey",
	"masterkey",
	"mysqlpwd",
	"nonce",
	"oauth",
	"oauthtoken",
	"otp",
	"passhash",
	"passwd",
	"password",
	"passwordb",
	"pemfile",
	"pgpkey",
	"phpsessid",
	"pin",
	"pincode",
	"pkcs8",
	"privatekey",
	"publickey",
	"pwd",
	"recaptchakey",
	"refreshtoken",
	"routingnumber",
	"salt",
	"secret",
	"secretkey",
	"secrettoken",
	"securityanswer",
	"securitycode",
	"securityquestion",
	"serviceaccountcredentials",
	"session",
	"sessionid",
	"sessionkey",
	"setcookie",
	"signature",
	"signaturekey",
	"sshkey",
	"ssn",
	"symfony",
	"token",
	"transactionid",
	"twiliotoken",
	"usersession",
	"voterid",
	"xapikey",
	"xauthtoken",
	"xcsrftoken",
	"xforwardedfor",
	"xrealip",
	"xsrf",
	"xsrftoken",
}

// Config is an immutable redaction policy.
type Config struct {
	identifiers map[string]struct{}
	typeExact   map[string]struct{}
	typePrefix  []string
}

// NewConfig builds a Config from the default keyword set plus the caller-supplied
// additions. extraIdentifiers are added to the defaults; excludedIdentifiers
// are removed from the result (and so can un-redact a default keyword);
// redactedTypes are matched by exact type name, or by prefix when the entry
// ends in "*" (or ".*"). All identifier inputs are normalized.
func NewConfig(extraIdentifiers, redactedTypes, excludedIdentifiers []string) *Config {
	ids := make(map[string]struct{}, len(defaultIdentifiers)+len(extraIdentifiers))
	for _, k := range defaultIdentifiers {
		ids[k] = struct{}{}
	}
	for _, k := range extraIdentifiers {
		if k = normalizeIdentifier(k); k != "" {
			ids[k] = struct{}{}
		}
	}
	for _, k := range excludedIdentifiers {
		if k = normalizeIdentifier(k); k != "" {
			delete(ids, k)
		}
	}
	c := &Config{identifiers: ids}
	for _, t := range redactedTypes {
		t = strings.TrimSpace(t)
		switch {
		case t == "":
			continue
		case strings.HasSuffix(t, "*"):
			// Drop only the "*", keeping any trailing "." so that
			// "pkg/auth.*" matches "pkg/auth.Token" but not "pkg/authz.Token".
			c.typePrefix = append(c.typePrefix, t[:len(t)-1])
		default:
			if c.typeExact == nil {
				c.typeExact = make(map[string]struct{})
			}
			c.typeExact[t] = struct{}{}
		}
	}
	return c
}

// RedactIdentifier reports whether a value held under the given variable,
// field, or map-key name must be redacted.
func (c *Config) RedactIdentifier(name string) bool {
	if c == nil {
		return false
	}
	_, ok := c.identifiers[normalizeIdentifier(name)]
	return ok
}

// RedactType reports whether a value of the given type name must be redacted.
func (c *Config) RedactType(typeName string) bool {
	if c == nil || typeName == "" {
		return false
	}
	if _, ok := c.typeExact[typeName]; ok {
		return true
	}
	for _, p := range c.typePrefix {
		if strings.HasPrefix(typeName, p) {
			return true
		}
	}
	return false
}

// normalizeIdentifier lowercases the name and removes the separators _ - $ @,
// matching the Java and .NET tracers. Other characters, including spaces, are
// preserved.
func normalizeIdentifier(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.TrimSpace(name) {
		switch r {
		case '_', '-', '$', '@':
			continue
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
