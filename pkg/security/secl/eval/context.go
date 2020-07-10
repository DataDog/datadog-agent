package eval

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	EmptyContext = &Context{}
)

// Context - Context used during the evaluation
type Context struct {
	Debug     bool
	evalDepth int
}

func (c *Context) Logf(format string, v ...interface{}) {
	log.Tracef(strings.Repeat("\t", c.evalDepth-1)+format, v...)
}
