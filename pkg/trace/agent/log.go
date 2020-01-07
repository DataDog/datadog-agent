// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package agent

import (
	"bytes"
	"strconv"
	"strings"
	"sync/atomic"
	"text/template"
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

// templateForwarder defines the template  used to forward any raw messages received by the throttled receiver.
var templateForwarder = template.Must(template.New("loggerForwarder").Parse(`
<seelog>
  <outputs formatid="raw">
	 {{- if .Console}}
	 <console />
	 {{- end}}
      <rollingfile type="size" filename="{{.FilePath}}" maxsize="10000000" maxrolls="5" />
  </outputs>
  <formats>
    <format id="raw" format="%Msg" />
  </formats>
</seelog>
`))

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

// templateError is the template used by the throttled receiver to report
// that maximum error capacity per the given interval has been reached.
var templateError = template.Must(template.New("loggerError").Parse(`
<seelog>
  {{- if .UseJSON}}
  <outputs formatid="json">
  {{- else}}
  <outputs formatid="common">
  {{- end}}
	 {{- if .Console}}
	 <console />
	 {{- end}}
      <rollingfile type="size" filename="{{.FilePath}}" maxsize="10000000" maxrolls="5" />
  </outputs>
  <formats>
    <format id="json" format="{{.JSONFormat}}"/>
    <format id="common" format="{{.CommonFormat}}"/>
  </formats>
</seelog>
`))

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
	logToConsole := args.XmlCustomAttrs["console"] == "true"

	cfgError := templateString(templateError, struct {
		UseJSON      bool
		FilePath     string
		Console      bool
		JSONFormat   string
		CommonFormat string
	}{
		FilePath:     logFilePath,
		Console:      logToConsole,
		UseJSON:      args.XmlCustomAttrs["use-json"] == "true",
		JSONFormat:   coreconfig.BuildJSONFormat(loggerName),
		CommonFormat: coreconfig.BuildCommonFormat(loggerName),
	})
	loggerError, err := seelog.LoggerFromConfigAsString(cfgError)
	if err != nil {
		return err
	}

	cfgForwarder := templateString(templateForwarder, struct {
		FilePath string
		Console  bool
	}{
		FilePath: logFilePath,
		Console:  logToConsole,
	})
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

// templateMain defines the main logger template.
var templateMain = template.Must(template.New("loggerMain").Parse(`
<seelog minlevel="{{.MinLevel}}">
  {{- if .UseJSON}}
  <outputs formatid="json">
  {{- else}}
  <outputs formatid="common">
  {{- end}}
    <filter levels="warn,error">
      <custom name="throttled" data-interval="{{if .Throttling}}10000000000{{else}}0{{end}}" data-console="{{if .Console}}true{{else}}false{{end}}" data-max-per-interval="10" data-use-json="{{.UseJSON}}" data-file-path="{{.FilePath}}" />
    </filter>
    <filter levels="trace,debug,info,critical">
      {{- if .Console}}
      <console />
      {{- end}}
      <rollingfile type="size" filename="{{.FilePath}}" maxsize="10000000" maxrolls="5" />
    </filter>
  </outputs>
  <formats>
    <format id="json" format="{{.JSONFormat}}"/>
    <format id="common" format="{{.CommonFormat}}"/>
  </formats>
</seelog>
`))

// setupLogger sets up the agent's logger based on the given agent configuration.
func setupLogger(cfg *config.AgentConfig) error {
	seelog.RegisterReceiver("throttled", &throttledReceiver{})

	logCfg := makeLoggerConfig(cfg)
	logger, err := seelog.LoggerFromConfigAsString(logCfg)
	if err != nil {
		return err
	}

	seelog.ReplaceLogger(logger)
	log.SetupDatadogLogger(logger, normalizeLogLevel(cfg.LogLevel))

	return nil
}

func normalizeLogLevel(lvl string) string {
	txt := strings.ToLower(lvl)
	if txt == "warning" {
		// to match core agent:
		// https://github.com/DataDog/datadog-agent/blob/6f2d901aeb19f0c0a4e09f149c7cc5a084d2f708/pkg/config/seelog.go#L74-L76
		txt = "warn"
	}
	level, ok := seelog.LogLevelFromString(txt)
	if !ok {
		level = seelog.InfoLvl
	}
	return level.String()
}

func makeLoggerConfig(cfg *config.AgentConfig) string {
	return templateString(templateMain, struct {
		MinLevel     string
		Throttling   bool
		UseJSON      bool
		FilePath     string
		Console      bool
		JSONFormat   string
		CommonFormat string
	}{
		MinLevel:     normalizeLogLevel(cfg.LogLevel),
		Throttling:   cfg.LogThrottling,
		UseJSON:      cfg.LogFormatJSON,
		FilePath:     cfg.LogFilePath,
		Console:      cfg.LogToConsole,
		JSONFormat:   coreconfig.BuildJSONFormat(loggerName),
		CommonFormat: coreconfig.BuildCommonFormat(loggerName),
	})
}

func templateString(t *template.Template, data interface{}) string {
	var buf bytes.Buffer
	t.Execute(&buf, data)
	return buf.String()
}
