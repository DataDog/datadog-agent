package filters

import (
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Blacklister holds a list of regular expressions which will match resources
// on spans that should be dropped.
type Blacklister struct {
	list []*regexp.Regexp
}

// Allows returns true if the Blacklister permits this span.
func (f *Blacklister) Allows(span *pb.Span) bool {
	for _, entry := range f.list {
		if entry.MatchString(span.Resource) {
			return false
		}
	}
	return true
}

// NewBlacklister creates a new Blacklister based on the given list of
// regular expressions.
func NewBlacklister(exprs []string) *Blacklister {
	return &Blacklister{list: compileRules(exprs)}
}

// compileRules compiles as many rules as possible from the list of expressions.
func compileRules(exprs []string) []*regexp.Regexp {
	list := make([]*regexp.Regexp, 0, len(exprs))
	for _, entry := range exprs {
		rule, err := regexp.Compile(entry)
		if err != nil {
			log.Errorf("Invalid resource filter: %q", entry)
			continue
		}
		list = append(list, rule)
	}
	return list
}
