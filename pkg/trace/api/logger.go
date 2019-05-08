package api

import (
	"sync"

	log "github.com/cihub/seelog"
)

// these could be configurable, but fine with hardcoded for now
var maxPerInterval int64 = 10

type errorLogger struct {
	errors int64
	sync.Mutex
}

func (l *errorLogger) Errorf(format string, params ...interface{}) {
	l.Lock()

	if l.errors < maxPerInterval {
		log.Errorf(format, params...)
	}
	if l.errors == maxPerInterval {
		log.Infof("too many error messages to display, skipping output till next minute")
	}

	l.errors++
	l.Unlock()
}

func (l *errorLogger) Reset() {
	l.Lock()
	if l.errors > maxPerInterval {
		log.Infof("skipped %d error messages", l.errors-maxPerInterval)
	}
	l.errors = 0
	l.Unlock()
}
