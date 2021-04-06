package snmp

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type TraceLevelLogWriter struct{}

func (sw *TraceLevelLogWriter) Write(p []byte) (n int, err error) {
	// TODO:
	//   - Scrub sensitive information
	//     - See strip_test.go
	log.Tracef(string(p))
	return len(p), nil
}
