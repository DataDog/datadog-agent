package agent

import (
	"fmt"
	"strconv"
	"time"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
)

const agentLoggerConfigFmt = `
<seelog minlevel="%[1]s">
  <outputs formatid="agent">
    <filter levels="warn,error">
      <custom name="throttled" data-interval="%[2]d" data-max-per-interval="%[3]d" data-file-path="%[4]s" />
    </filter>
    <filter levels="trace,debug,info,critical">
      <console />
      <rollingfile type="size" filename="%[4]s" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="agent" format="%%Date %%Time %%LEVEL (%%File:%%Line) - %%Msg%%n" />
  </formats>
</seelog>
`

const rawLoggerConfigFmt = `
<seelog>
  <outputs formatid="agent">
      <console />
      <rollingfile type="size" filename="%s" maxsize="10000000" maxrolls="5" />
  </outputs>
  <formats>
    <format id="agent" format="%%Date %%Time %%LEVEL (%%File:%%Line) - %%Msg%%n" />
  </formats>
</seelog>
`

const rawLoggerNoFmtConfigFmt = `
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

// forwardLogMsg forwards the given message to the given logger making
// sure the log level is kept.
func forwardLogMsg(logger log.LoggerInterface, msg string, lvl log.LogLevel) {
	switch lvl {
	case log.TraceLvl:
		logger.Trace(msg)
	case log.DebugLvl:
		logger.Debug(msg)
	case log.InfoLvl:
		logger.Info(msg)
	case log.WarnLvl:
		logger.Warn(msg)
	case log.ErrorLvl:
		logger.Error(msg)
	case log.CriticalLvl:
		logger.Critical(msg)
	}
}

// ThrottledReceiver is a custom seelog receiver dropping log messages
// once the maximum number of log messages per interval have been
// reached.
// NOTE: we don't need to protect our log counter with a
// mutex. Seelog's default logger type is the asynchronous loop
// logger, implemented as a goroutine processing logs independently
// from where they were emitted
// (https://github.com/cihub/seelog/wiki/Logger-types).
type ThrottledReceiver struct {
	maxLogsPerInterval int64

	rawLogger      log.LoggerInterface
	rawLoggerNoFmt log.LoggerInterface

	logCount int64
	tick     <-chan time.Time
	done     chan struct{}
}

// ReceiveMessage implements log.CustomReceiver
func (r *ThrottledReceiver) ReceiveMessage(msg string, lvl log.LogLevel, _ log.LogContextInterface) error {
	r.logCount++

	if r.maxLogsPerInterval < 0 || r.logCount < r.maxLogsPerInterval {
		forwardLogMsg(r.rawLoggerNoFmt, msg, lvl)
	} else if r.logCount == r.maxLogsPerInterval {
		r.rawLogger.Error("Too many messages to log, skipping for a bit...")
	}
	return nil
}

// AfterParse implements log.CustomReceiver
func (r *ThrottledReceiver) AfterParse(args log.CustomReceiverInitArgs) error {
	// Parse the maxLogs attribute (no verification needed, its an
	// integer for sure)
	interval, _ := strconv.Atoi(args.XmlCustomAttrs["interval"])

	// Parse the maxLogs attribute (no verification needed, its an
	// integer for sure)
	maxLogsPerInterval, _ := strconv.Atoi(
		args.XmlCustomAttrs["max-per-interval"],
	)

	// Parse the logFilePath attribute
	logFilePath := args.XmlCustomAttrs["file-path"]

	// Setup rawLogger
	rawLoggerConfig := fmt.Sprintf(rawLoggerConfigFmt, logFilePath)
	rawLogger, err := log.LoggerFromConfigAsString(rawLoggerConfig)
	if err != nil {
		return err
	}

	// Setup rawLoggerNoFmt
	rawLoggerNoFmtConfig := fmt.Sprintf(rawLoggerNoFmtConfigFmt, logFilePath)
	rawLoggerNoFmt, err := log.LoggerFromConfigAsString(rawLoggerNoFmtConfig)
	if err != nil {
		return err
	}

	// Setup the ThrottledReceiver
	r.maxLogsPerInterval = int64(maxLogsPerInterval)
	r.rawLogger = rawLogger
	r.rawLoggerNoFmt = rawLoggerNoFmt
	r.done = make(chan struct{})

	// If no interval was given, no need to continue setup
	if interval <= 0 {
		r.maxLogsPerInterval = -1
		return nil
	}

	r.logCount = 0
	r.tick = time.Tick(time.Duration(interval))

	// Start the goroutine resetting the log count
	go func() {
		defer watchdog.LogOnPanic()
		for {
			select {
			case <-r.tick:
				r.logCount = 0
			case <-r.done:
				return
			}
		}

	}()

	return nil
}

// Flush implements log.CustomReceiver
func (r *ThrottledReceiver) Flush() {
	// Flush all raw loggers, a typical use cases for log is showing an error at startup
	// (eg: "cannot listen on localhost:8126: listen tcp 127.0.0.1:8126: bind: address already in use")
	// and those are not shown if we don't Flush for real.
	if r.rawLogger != nil { // set by AfterParse, so double-checking it's not nil
		r.rawLogger.Flush()
	}
	if r.rawLoggerNoFmt != nil { // set by AfterParse, so double-checking it's not nil
		r.rawLoggerNoFmt.Flush()
	}
}

// Close implements log.CustomReceiver
func (r *ThrottledReceiver) Close() error {
	// Stop the go routine periodically resetting the log count
	close(r.done)
	return nil
}

// SetupLogger sets up the agent's logger. We use seelog for logging
// in the following way:
// * Logs with a level under "minLogLvl" are dropped.
// * Logs with a level of "trace", "debug" and "info" are always
//   showed if "minLogLvl" is set accordingly. This is for development
//   purposes.
// * Logs with a level of "warn" or "error" are dropped after
//   "logsDropMaxPerInterval" number of messages are showed. The
//   counter is reset every "logsDropInterval". If "logsDropInterval"
//   is 0, dropping is disabled (and might flood your logs!).
func SetupLogger(minLogLvl log.LogLevel, logFilePath string, logsDropInterval time.Duration, logsDropMaxPerInterval int) error {
	log.RegisterReceiver("throttled", &ThrottledReceiver{})

	// Build our config string
	logConfig := fmt.Sprintf(
		agentLoggerConfigFmt,
		minLogLvl,
		logsDropInterval,
		logsDropMaxPerInterval,
		logFilePath,
	)

	logger, err := log.LoggerFromConfigAsString(logConfig)
	if err != nil {
		return err
	}
	return log.ReplaceLogger(logger)
}

// SetupDefaultLogger sets up a default logger for the agent, showing
// all log messages and with no throttling.
func SetupDefaultLogger() error {
	logConfig := fmt.Sprintf(rawLoggerConfigFmt, config.DefaultLogFilePath)

	logger, err := log.LoggerFromConfigAsString(logConfig)
	if err != nil {
		return err
	}
	return log.ReplaceLogger(logger)
}
