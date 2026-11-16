// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(CAPP) Fix revive linter
package redact

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var forbiddenSymbols = regexp.MustCompile("[^a-zA-Z0-9_*]")

const (
	// regexSensitiveParamInJSON is using non greedy operators in the value capture
	// group to work around missing support for look behind assertions in Go.
	// Keys are matched case-insensitively, e.g. "authKey" for the "authkey" word.
	regexSensitiveParamInJSON = `(?P<before_value>"(?i:%s)"\s*:\s*)(?P<value>".*?[^\\]+?")`

	redactedAnnotationValue = "-"
	redactedSecret          = "********"
)

var (
	defaultSensitiveWords = []string{
		"password", "passwd", "mysql_pwd",
		"access_token", "auth_token",
		"api_key", "apikey", "pwd",
		"secret", "credentials", "stripetoken",
		// SNMP credentials\
		"community_string", "authkey", "auth_key", "privkey", "priv_key", "authentication_key", "privacy_key"}

	knownSafeEnvVars = map[string]struct{}{
		"DD_AUTH_TOKEN_FILE_PATH": {},
	}

	// cmdlineTokenizer tokenizes by capturing non-whitespace terms as tokens EX: "agent --secret" > ["agent", "--secret"]
	// and non-whitespace terms followed by quotation enclosed subcomponents as tokens EX: "agent --pass="secret house"" > ["agent", "--pass="secret house""]
	cmdlineTokenizer = regexp.MustCompile(`([^\s"']+("([^"]*)")*('([^']*)')*)`)

	// knownSecretFormats are credentials recognized by their own format, so they are
	// redacted even when the surrounding key or env var name gives no hint (e.g. a JWT
	// in USER_IDENTITY). Only formats with a distinctive prefix are listed, so hashes
	// and IDs are left alone. Most patterns are adapted from the gitleaks default rules
	// (https://github.com/gitleaks/gitleaks/blob/master/config/gitleaks.toml, MIT).
	// Each format lists literals every match contains: almost no value contains one,
	// so checking them first skips the regex in the common case.
	knownSecretFormats = []struct {
		pattern string
		markers []string
	}{
		{`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`, []string{"eyJ"}},                                                                 // JWT
		{`\bgh[pousr]_[A-Za-z0-9]{36,}`, []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}},                                                                // GitHub token
		{`\bgithub_pat_[A-Za-z0-9_]{22,}`, []string{"github_pat_"}},                                                                                       // GitHub fine-grained token
		{`\bglp[at]t-[A-Za-z0-9_-]{20,}`, []string{"glpat-", "glptt-"}},                                                                                   // GitLab personal and trigger tokens
		{`\bxox[abposre]-[A-Za-z0-9-]{10,}`, []string{"xox"}},                                                                                             // Slack token
		{`\bxapp-[0-9]+-[A-Za-z0-9-]{10,}`, []string{"xapp-"}},                                                                                            // Slack app token
		{`\bnpm_[A-Za-z0-9]{36}`, []string{"npm_"}},                                                                                                       // npm token
		{`\bsk-ant-(?:api03|admin01)-[A-Za-z0-9_-]{93}AA`, []string{"sk-ant-"}},                                                                           // Anthropic API key
		{`\bsk-(?:(?:proj|svcacct|admin)-[A-Za-z0-9_-]{58,74}|[A-Za-z0-9]{20})T3BlbkFJ[A-Za-z0-9_-]{20,74}`, []string{"T3BlbkFJ"}},                        // OpenAI API key
		{`\b(?:sk|rk)_(?:test|live|prod)_[A-Za-z0-9]{10,99}`, []string{"k_test_", "k_live_", "k_prod_"}},                                                  // Stripe key
		{`\bSG\.[A-Za-z0-9=_.-]{66}`, []string{"SG."}},                                                                                                    // SendGrid API key
		{`\bAIza[A-Za-z0-9_-]{35}`, []string{"AIza"}},                                                                                                     // Google API key
		{`\b(?:hf|api_org)_[A-Za-z]{34}`, []string{"hf_", "api_org_"}},                                                                                    // Hugging Face token
		{`\bhv[sb]\.[A-Za-z0-9_-]{90,300}`, []string{"hvs.", "hvb."}},                                                                                     // HashiCorp Vault token
		{`\bdapi[a-f0-9]{32}(?:-[0-9])?`, []string{"dapi"}},                                                                                               // Databricks token
		{`\bdo[opr]_v1_[a-f0-9]{64}`, []string{"_v1_"}},                                                                                                   // DigitalOcean token
		{`\b(?:glsa_[A-Za-z0-9]{32}_[A-Fa-f0-9]{8}|glc_[A-Za-z0-9+/]{32,400}={0,3})`, []string{"glsa_", "glc_"}},                                          // Grafana token
		{`\bsntry[us]_[A-Za-z0-9+/=_]{64,}`, []string{"sntryu_", "sntrys_"}},                                                                              // Sentry token
		{`\blin_api_[A-Za-z0-9]{40}`, []string{"lin_api_"}},                                                                                               // Linear API key
		{`\bNRAK-[A-Za-z0-9]{27}`, []string{"NRAK-"}},                                                                                                     // New Relic user API key
		{`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,1000}`, []string{"pypi-AgEIcHlwaS5vcmc"}},                                                                // PyPI upload token
		{`\bshp(?:at|ca|pa|ss)_[a-fA-F0-9]{32}`, []string{"shpat_", "shpca_", "shppa_", "shpss_"}},                                                        // Shopify token
		{`\bdp\.pt\.[A-Za-z0-9]{43}`, []string{"dp.pt."}},                                                                                                 // Doppler token
		{`\bPMAK-[a-fA-F0-9]{24}-[a-fA-F0-9]{34}`, []string{"PMAK-"}},                                                                                     // Postman API key
		{`\bATATT3[A-Za-z0-9_=-]{186}`, []string{"ATATT3"}},                                                                                               // Atlassian API token
		{`\b[A-Za-z0-9_~.]{3}[0-9]Q~[A-Za-z0-9_~.-]{31,34}`, []string{"Q~"}},                                                                              // Azure AD client secret
		{`-----BEGIN[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----[\s\S]*?-----END[ A-Z0-9_-]{0,100}PRIVATE KEY(?: BLOCK)?-----`, []string{"PRIVATE KEY"}}, // PEM or PGP private key
	}

	knownSecretValues = compileKnownSecretFormats()

	// uriCredentials matches the password in "scheme://user:password@host" (as the
	// default agent scrubber does), but requires a scheme so image references such as
	// "nginx:1.25@sha256:..." are kept. Passwords can't hold "/": RFC 3986 requires
	// it to be percent-encoded.
	uriCredentials = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^:\s/@"]*:)[^\s"@/]+@`)

	// bearerCredentials matches the token in an HTTP "Authorization: Bearer <token>".
	bearerCredentials = regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
)

// secretKind says which regex a marker calls for.
type secretKind uint8

const (
	knownFormatSecret secretKind = 1 << iota
	uriCredentialsSecret
	bearerSecret
)

// secretMarkersByFirstByte indexes every marker by its first byte, so a value is
// checked for all of them in a single pass.
var secretMarkersByFirstByte [256][]struct {
	text string
	kind secretKind
}

// compileKnownSecretFormats builds one regex matching any known secret format, and
// indexes the markers of every regex.
func compileKnownSecretFormats() *regexp.Regexp {
	addMarker := func(text string, kind secretKind) {
		secretMarkersByFirstByte[text[0]] = append(secretMarkersByFirstByte[text[0]], struct {
			text string
			kind secretKind
		}{text, kind})
	}
	patterns := make([]string, 0, len(knownSecretFormats))
	for _, f := range knownSecretFormats {
		patterns = append(patterns, f.pattern)
		for _, m := range f.markers {
			addMarker(m, knownFormatSecret)
		}
	}
	addMarker("://", uriCredentialsSecret)
	for _, m := range []string{"Bearer", "bearer", "BEARER"} {
		addMarker(m, bearerSecret)
	}
	return regexp.MustCompile(strings.Join(patterns, "|"))
}

// secretKindsIn returns the kinds of secrets whose markers s contains.
func secretKindsIn(s string) secretKind {
	var kinds secretKind
	for i := 0; i < len(s); i++ {
		for _, m := range secretMarkersByFirstByte[s[i]] {
			if strings.HasPrefix(s[i:], m.text) {
				kinds |= m.kind
			}
		}
	}
	return kinds
}

// DataScrubber allows the agent to block cmdline arguments that match
// a list of predefined and custom words
type DataScrubber struct {
	Enabled bool
	// RegexSensitivePatterns are custom regex patterns which are currently not exposed externally
	RegexSensitivePatterns []*regexp.Regexp
	// LiteralSensitivePatterns are custom words which use to match against
	LiteralSensitivePatterns         []string
	regexSensitiveWordsInAnnotations []*regexp.Regexp
}

// NewDefaultDataScrubber creates a DataScrubber with the default behavior: enabled
// and matching the default sensitive words
func NewDefaultDataScrubber() *DataScrubber {
	newDataScrubber := &DataScrubber{
		Enabled:                  true,
		LiteralSensitivePatterns: defaultSensitiveWords,
	}

	newDataScrubber.setupAnnotationRegexps(defaultSensitiveWords)

	return newDataScrubber
}

func (ds *DataScrubber) setupAnnotationRegexps(words []string) {
	for _, word := range words {
		r := regexp.MustCompile(fmt.Sprintf(regexSensitiveParamInJSON, regexp.QuoteMeta(word)))
		ds.regexSensitiveWordsInAnnotations = append(ds.regexSensitiveWordsInAnnotations, r)
	}
}

// ContainsSensitiveWord returns true if the given string contains
// a sensitive word
func (ds *DataScrubber) ContainsSensitiveWord(s string) bool {
	for _, pattern := range ds.LiteralSensitivePatterns {
		if strings.Contains(strings.ToLower(s), pattern) {
			if _, ok := knownSafeEnvVars[s]; ok {
				return false
			}
			return true
		}
	}
	return false
}

// ScrubAnnotationValue obfuscate sensitive information from an annotation
// value.
func (ds *DataScrubber) ScrubAnnotationValue(annotationValue string) string {
	for _, r := range ds.regexSensitiveWordsInAnnotations {
		if r.MatchString(annotationValue) {
			annotationValue = r.ReplaceAllString(annotationValue, fmt.Sprintf(`${before_value}"%s"`, redactedSecret))
		}
	}
	return ds.ScrubKnownSecretValues(annotationValue)
}

// ScrubKnownSecretValues replaces credentials recognized by their format (see
// knownSecretFormats), URL passwords and bearer tokens with ********, keeping the
// rest of the string: "Bearer eyJ..." becomes "Bearer ********".
func (ds *DataScrubber) ScrubKnownSecretValues(s string) string {
	kinds := secretKindsIn(s)
	if kinds == 0 {
		return s
	}
	if kinds&knownFormatSecret != 0 {
		s = knownSecretValues.ReplaceAllLiteralString(s, redactedSecret)
	}
	if kinds&uriCredentialsSecret != 0 {
		s = uriCredentials.ReplaceAllString(s, "${1}"+redactedSecret+"@")
	}
	if kinds&bearerSecret != 0 {
		s = bearerCredentials.ReplaceAllString(s, "${1}"+redactedSecret)
	}
	return s
}

// ScrubSimpleCommand hides the argument value for any key which matches a "sensitive word" pattern.
// It returns the updated cmdline, as well as a boolean representing whether it was scrubbed.
func (ds *DataScrubber) ScrubSimpleCommand(cmd, args []string) ([]string, []string, bool) {
	if len(cmd) == 0 && len(args) == 0 {
		return cmd, args, false
	}

	changed := false
	regexChanged := false
	var wordReplacesIndexes []int

	separator := "cmd_separator"
	cmdline := append(cmd, separator)
	cmdline = append(cmdline, args...)

	// in case we have custom regexes we have to join them and perform regex find and replace
	rawCmdline := strings.Join(cmdline, " ")
	for _, pattern := range ds.RegexSensitivePatterns {
		if pattern.MatchString(rawCmdline) {
			regexChanged = true
			rawCmdline = pattern.ReplaceAllString(rawCmdline, "${key}${delimiter}"+redactedSecret)
		}
	}

	newCmdline := cmdlineTokenizer.FindAllString(rawCmdline, -1)

	// remove the separator from the cmdline
	cmIndex := 0
	for i := 0; i < len(newCmdline); i++ {
		if newCmdline[i] == separator {
			cmIndex = i
			break
		}
	}
	if cmIndex == len(newCmdline)-1 {
		newCmdline = newCmdline[:cmIndex]
	} else {
		newCmdline = append(newCmdline[:cmIndex], newCmdline[cmIndex+1:]...)
	}

	// preprocess, without the preprocessing we would need to strip until whitespaces.
	// the first index can be skipped because it should be the program name.
	for index := 1; index < len(newCmdline); index++ {
		cmd := newCmdline[index]
		lowerCmd := strings.ToLower(cmd)
		for _, pattern := range ds.LiteralSensitivePatterns {
			// if we found a word from the list,
			// it means either:
			// - the current after a delimiter should be a password we want to replace e.g. "agent --secret=<replace-me>" (1)
			// - the next word should be replaced.  e.g. "agent --secret <replace-me>" (1)
			// - the word is part of a special list of words, contains multiple supportedEnds and can be ignored e.g. "agent > /secret/secret" should not match (3)
			matchIndex := strings.Index(lowerCmd, pattern)
			// password<delimiter>1234 || password || (ignore<delimiter>me<delimiter>password e.g ignore:me:password ignore/me/password)
			// agent --password======test
			// agent > /password/secret ==> agent > /password/secret
			// agent --password > /password/secret ==> agent > --password ******** /password/secret

			if matchIndex >= 0 {
				before := cmd[:matchIndex] // /etc/vaultd/ from /etc/vaultd/secret/haproxy-crt.pem
				// skip paths /etc/vaultd/secrets/haproxy-crt.pem -> we don't want to match if one of the below chars are in before
				if strings.ContainsAny(before, "/:=$") {
					break
				}

				changed = true
				v := strings.IndexAny(cmd, "=:")
				if v >= 0 {
					// password:1234  password=1234 ==> password=****** || password:******
					// password::::====1234 ==> password:******
					newCmdline[index] = cmd[:v+1] + redactedSecret
					// replace from v to end of string with ********
					break
				} else { //nolint:revive // TODO(CAPP) Fix revive linter
					// password 1234 password ******
					nextReplacementIndex := index + 1
					if nextReplacementIndex < len(newCmdline) {
						wordReplacesIndexes = append(wordReplacesIndexes, nextReplacementIndex)
						index++
					}
					break
				}
			}
		}
	}

	for _, index := range wordReplacesIndexes {
		// we only want to replace words, hence we jump to the next index and try to scrub that instead
		for index < len(newCmdline) && newCmdline[index] == "" {
			index++
		}
		if index < len(newCmdline) {
			newCmdline[index] = redactedSecret
		}
	}

	// if nothing changed, just return the input
	if !(changed || regexChanged) {
		return cmd, args, false
	}

	return newCmdline[:cmIndex], newCmdline[cmIndex:], changed || regexChanged
}

// AddCustomSensitiveWords adds custom sensitive words on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveWords(words []string) {
	ds.LiteralSensitivePatterns = append(ds.LiteralSensitivePatterns, words...)
	ds.setupAnnotationRegexps(words)
}

// AddCustomSensitiveRegex adds custom sensitive regex on the DataScrubber object
func (ds *DataScrubber) AddCustomSensitiveRegex(words []string) {
	r := compileStringsToRegex(words)
	ds.RegexSensitivePatterns = append(ds.RegexSensitivePatterns, r...)
}

// compileStringsToRegex compile each word in the slice into a regex pattern to match
// against the cmdline arguments (originally imported from pkg/process/config)
// The word must contain only word characters ([a-zA-z0-9_]) or wildcards *
func compileStringsToRegex(words []string) []*regexp.Regexp {
	compiledRegexps := make([]*regexp.Regexp, 0, len(words))

	for _, word := range words {
		if forbiddenSymbols.MatchString(word) {
			log.Warnf("data scrubber: %s skipped. The sensitive word must "+
				"contain only alphanumeric characters, underscores or wildcards ('*')", word)
			continue
		}

		if word == "*" {
			log.Warn("data scrubber: ignoring wildcard-only ('*') sensitive word as it is not supported")
			continue
		}

		originalRunes := []rune(word)
		var enhancedWord bytes.Buffer
		valid := true
		for i, rune := range originalRunes {
			if rune == '*' {
				if i == len(originalRunes)-1 {
					enhancedWord.WriteString("[^ =:]*")
				} else if originalRunes[i+1] == '*' {
					log.Warnf("data scrubber: %s skipped. The sensitive word "+
						"must not contain two consecutives '*'", word)
					valid = false
					break
				} else {
					enhancedWord.WriteString("[^\\s=:$/]*")
				}
			} else {
				enhancedWord.WriteString(string(rune))
			}
		}

		if !valid {
			continue
		}

		pattern := "(?P<key>( +| -{1,2})(?i)" + enhancedWord.String() + ")(?P<delimiter> +|=|:)(?P<value>[^\\s]*)"
		r, err := regexp.Compile(pattern)
		if err == nil {
			compiledRegexps = append(compiledRegexps, r)
		} else {
			log.Warnf("data scrubber: %s skipped. It couldn't be compiled into a regex expression", word)
		}
	}

	return compiledRegexps
}
