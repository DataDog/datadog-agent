package procmatch

import (
	"strings"
)

type containsMatcher struct {
	signatures []signature
}

func (m containsMatcher) Match(cmdline string) string {
	normalized := strings.ToLower(cmdline)

	for _, s := range m.signatures {
		if s.match(normalized) {
			return s.integration
		}
	}

	return ""
}

// NewWithContains builds a contains matcher from an integration catalog
func NewWithContains(catalog IntegrationCatalog) Matcher {
	signatures := buildSignatures(catalog)
	return containsMatcher{signatures}
}
