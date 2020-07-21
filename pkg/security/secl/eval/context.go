package eval

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Context describes the context used during a rule evaluation
type Context struct {
	Debug     bool
	evalDepth int
}

// Logf formats according to a format specifier and outputs to the current logger
func (c *Context) Logf(format string, v ...interface{}) {
	log.Tracef(strings.Repeat("\t", c.evalDepth-1)+format, v...)
}
