// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package agent

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
)

// throttledReceiver is a custom seelog receiver dropping log messages
// once the maximum number of log messages per interval have been
// reached.
// NOTE: we don't need to protect our log counter with a
// mutex. Seelog's default logger type is the asynchronous loop
// logger, implemented as a goroutine processing logs independently
// from where they were emitted
// (https://github.com/cihub/seelog/wiki/Logger-types).
type throttledReceiver struct {
	logCount           uint64
	maxLogsPerInterval uint64

	loggerError     seelog.LoggerInterface
	loggerForwarder seelog.LoggerInterface

	done chan struct{}
}

// loggerName is the name of the trace agent logger
const loggerName coreconfig.LoggerName = "TRACE"

// loggerConfigForwarder is used to forward any raw messages received by the throttled
// receiver.
const loggerConfigForwarder = `
<seelog>
  <outputs formatid="raw">
      <console />
      <rollingfile type="size" filename="%s" maxsize="10000000" maxrolls="5" />
  </outputs>
  <formats>
    <format id="raw" format="%%Msg" />
  </formats>
</seelog>
`

// ReceiveMessage implements seelog.CustomReceiver
func (r *throttledReceiver) ReceiveMessage(msg string, lvl seelog.LogLevel, _ seelog.LogContextInterface) error {
	logCount := atomic.AddUint64(&r.logCount, 1)

	if r.maxLogsPerInterval == 0 || logCount < r.maxLogsPerInterval {
		r.forwardLogMsg(msg, lvl)
	} else if logCount == r.maxLogsPerInterval {
		r.loggerError.Error("Too many messages to log, skipping for a bit...")
	}
	return nil
}

// loggerConfigError is the template used by the throttled receiver to report
// that maximum error capacity per the given interval has been reached.
const loggerConfigError = `
<seelog>
  <outputs formatid="%[1]s">
      <console />
      <rollingfile type="size" filename="%[2]s" maxsize="10000000" maxrolls="5" />
  </outputs>
  <formats>
    <format id="json" format="%[3]s"/>
    <format id="common" format="%[4]s"/>
  </formats>
</seelog>
`

// forwardLogMsg forwards the given message to the given logger making
// sure the log level is kept.
func (r *throttledReceiver) forwardLogMsg(msg string, lvl seelog.LogLevel) {
	switch lvl {
	case seelog.TraceLvl:
		r.loggerForwarder.Trace(msg)
	case seelog.DebugLvl:
		r.loggerForwarder.Debug(msg)
	case seelog.InfoLvl:
		r.loggerForwarder.Info(msg)
	case seelog.WarnLvl:
		r.loggerForwarder.Warn(msg)
	case seelog.ErrorLvl:
		r.loggerForwarder.Error(msg)
	case seelog.CriticalLvl:
		r.loggerForwarder.Critical(msg)
	}
}

// AfterParse implements seelog.CustomReceiver
func (r *throttledReceiver) AfterParse(args seelog.CustomReceiverInitArgs) error {
	interval, err := strconv.Atoi(args.XmlCustomAttrs["interval"])
	if err != nil {
		return err
	}
	maxLogsPerInterval, err := strconv.ParseUint(args.XmlCustomAttrs["max-per-interval"], 10, 64)
	if err != nil {
		return err
	}
	logFilePath := args.XmlCustomAttrs["file-path"]
	format := "common"
	if args.XmlCustomAttrs["use-json"] == "true" {
		format = "json"
	}

	cfgError := fmt.Sprintf(
		loggerConfigError,
		format,
		logFilePath,
		coreconfig.BuildJSONFormat(loggerName),
		coreconfig.BuildCommonFormat(loggerName),
	)
	loggerError, err := seelog.LoggerFromConfigAsString(cfgError)
	if err != nil {
		return err
	}

	cfgForwarder := fmt.Sprintf(loggerConfigForwarder, logFilePath)
	loggerForwarder, err := seelog.LoggerFromConfigAsString(cfgForwarder)
	if err != nil {
		return err
	}

	r.maxLogsPerInterval = maxLogsPerInterval
	r.loggerError = loggerError
	r.loggerForwarder = loggerForwarder
	r.done = make(chan struct{})

	// If no interval was given, no need to continue setup
	if interval <= 0 {
		r.maxLogsPerInterval = 0
		return nil
	}

	atomic.SwapUint64(&r.logCount, 0)
	tick := time.Tick(time.Duration(interval))

	// Start the goroutine resetting the log count
	go func() {
		defer watchdog.LogOnPanic()
		for {
			select {
			case <-tick:
				atomic.SwapUint64(&r.logCount, 0)
			case <-r.done:
				return
			}
		}

	}()

	return nil
}

// Flush implements seelog.CustomReceiver
func (r *throttledReceiver) Flush() {
	// Flush all raw loggers, a typical use cases for log is showing an error at startup
	// (eg: "cannot listen on localhost:8126: listen tcp 127.0.0.1:8126: bind: address already in use")
	// and those are not shown if we don't Flush for real.
	if r.loggerError != nil { // set by AfterParse, so double-checking it's not nil
		r.loggerError.Flush()
	}
	if r.loggerForwarder != nil { // set by AfterParse, so double-checking it's not nil
		r.loggerForwarder.Flush()
	}
}

// Close implements seelog.CustomReceiver
func (r *throttledReceiver) Close() error {
	// Stop the go routine periodically resetting the log count
	close(r.done)
	return nil
}

// loggerConfig specifies the main agent configuration.
const loggerConfig = `
<seelog minlevel="%[1]s">
  <outputs formatid="%[2]s">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="%[3]d" data-max-per-interval="10" data-use-json="%[4]v" data-file-path="%[5]s" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <console />
      <rollingfile type="size" filename="%[5]s" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="%[6]s"/>
    <format id="common" format="%[7]s"/>
  </formats>
</seelog>
`

// setupLogger sets up the agent's logger based on the given agent configuration.
func setupLogger(cfg *config.AgentConfig) error {
	logLevel := strings.ToLower(cfg.LogLevel)
	if logLevel == "warning" {
		// to match core agent:
		// https://github.com/DataDog/datadog-agent/blob/6f2d901aeb19f0c0a4e09f149c7cc5a084d2f708/pkg/config/seelog.go#L74-L76
		logLevel = "warn"
	}
	minLogLvl, ok := seelog.LogLevelFromString(logLevel)
	if !ok {
		minLogLvl = seelog.InfoLvl
	}
	var duration time.Duration
	if cfg.LogThrottling {
		duration = 10 * time.Second
	}
	format := "common"
	if coreconfig.Datadog.GetBool("log_format_json") {
		format = "json"
	}

	seelog.RegisterReceiver("throttled", &throttledReceiver{})

	logConfig := fmt.Sprintf(
		loggerConfig,
		minLogLvl,
		format,
		duration,
		format == "json",
		cfg.LogFilePath,
		coreconfig.BuildJSONFormat(loggerName),
		coreconfig.BuildCommonFormat(loggerName),
	)
	logger, err := seelog.LoggerFromConfigAsString(logConfig)
	if err != nil {
		return err
	}

	seelog.ReplaceLogger(logger)
	log.SetupDatadogLogger(logger, minLogLvl.String())

	return nil
}
