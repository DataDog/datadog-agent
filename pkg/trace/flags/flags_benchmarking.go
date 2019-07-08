// +build benchmarking

package flags

import (
	"flag"
	"fmt"
	"time"
)

// StatsOut specifies the file to write metrics to.
var StatsOut string

func init() {
	flag.StringVar(&StatsOut, "stats-out", fmt.Sprintf("metrics-%s.stats", time.Now().Format("02-01-2006-15:04:05")), "file to write metrics to")
}
