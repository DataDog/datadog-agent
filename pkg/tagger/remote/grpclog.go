package remote

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc/grpclog"
)

type logLevel int

const (
	logLevelInfo logLevel = iota
	logLevelWarn
	logLevelErr
)

type redirectLogger struct {
	level logLevel
}

func newRedirectLogger(level logLevel) redirectLogger {
	return redirectLogger{level: level}
}

func (l redirectLogger) Write(b []byte) (int, error) {
	msg := string(b)

	switch l.level {
	case logLevelInfo:
		log.InfoStackDepth(stackDepth, msg)
	case logLevelWarn:
		log.WarnStackDepth(stackDepth, msg)
	case logLevelErr:
		log.ErrorStackDepth(stackDepth, msg)
	default:
		log.InfoStackDepth(stackDepth, msg)
	}

	return 0, nil
}

const stackDepth = 7

func init() {
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(
		newRedirectLogger(logLevelInfo),
		newRedirectLogger(logLevelWarn),
		newRedirectLogger(logLevelErr),
	))
}
