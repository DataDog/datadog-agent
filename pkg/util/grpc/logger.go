// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"io"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc/grpclog"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	stackDepth      = 7
	timestampOffset = 22
)

type redirectLogger struct {
}

func newRedirectLogger() redirectLogger {
	return redirectLogger{}
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

	switch b[0] {
	case 'I':
		log.InfoStackDepth(stackDepth, msg)
	case 'W':
		log.WarnStackDepth(stackDepth, msg)
	case 'E':
		log.ErrorStackDepth(stackDepth, msg)
	case 'F':
		log.CriticalStackDepth(stackDepth, msg)
	default:
		log.InfoStackDepth(stackDepth, msg)
	}

	return 0, nil
}

// NewLogger returns a gRPC logger that logs to the Datadog logger instead of
// directly to stderr.
func NewLogger() grpclog.LoggerV2 {
	errorW := io.Discard
	warningW := io.Discard
	infoW := io.Discard

	// grpc-go logs to an io.Writer on a certain severity, and every single
	// lower severity (so FATAL would log to FATAL, ERROR, WARN and INFO),
	// causing a lot of duplication, and that's why we parse the severity
	// out of the log instead of having an output for each severity

	logLevel := strings.ToLower(os.Getenv("GRPC_GO_LOG_SEVERITY_LEVEL"))
	switch logLevel {
	case "", "error": // If env is unset, set level to ERROR.
		errorW = newRedirectLogger()
	case "warning":
		warningW = newRedirectLogger()
	case "info":
		infoW = newRedirectLogger()
	}

	var v int
	vLevel := os.Getenv("GRPC_GO_LOG_VERBOSITY_LEVEL")
	if vl, err := strconv.Atoi(vLevel); err == nil {
		v = vl
	}

	return grpclog.NewLoggerV2WithVerbosity(infoW, warningW, errorW, v)
}
