package remote

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"google.golang.org/grpc/grpclog"
)

const timestampOffset = 22

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
	// Write receives an already formatted log line, so we need to parse it
	// to remove bits that would be duplicated in the datadog logger.
	// For example: `INFO: 2021/02/04 14:06:11 parsed scheme: ""`

	msg := string(b)

	// the log level is the only variable length substring we need to take
	// into account. timestampOffset is the length of the timestamp itself
	// plus extra spacing characters
	levelSepIndex := strings.Index(msg, ":")
	msg = msg[levelSepIndex+timestampOffset:]

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
