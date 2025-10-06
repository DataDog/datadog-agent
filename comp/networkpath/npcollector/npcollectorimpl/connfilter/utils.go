package connfilter

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	allowedWildcardMatchPattern = regexp.MustCompile(`^[a-zA-Z0-9\-_*.]+$`)
)

func buildRegex(matchRe string, matchType matchDomainStrategyType) (*regexp.Regexp, error) {
	if matchType == matchDomainStrategyWildcard {
		if !allowedWildcardMatchPattern.MatchString(matchRe) {
			return nil, fmt.Errorf("invalid wildcard match pattern `%s`, it does not match allowed match regex `%s`", matchRe, allowedWildcardMatchPattern)
		}
		if strings.Contains(matchRe, "**") {
			return nil, fmt.Errorf("invalid wildcard match pattern `%s`, it should not contain consecutive `*`", matchRe)
		}
		matchRe = strings.ReplaceAll(matchRe, ".", "\\.")
		// TODO: why do we need ^ in "([^.]*)" ??
		matchRe = strings.ReplaceAll(matchRe, "*", ".*")
	}
	regex, err := regexp.Compile("^" + matchRe + "$")
	if err != nil {
		return nil, fmt.Errorf("invalid match `%s`. cannot compile regex: %v", matchRe, err)
	}
	return regex, nil
}
