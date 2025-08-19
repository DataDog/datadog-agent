// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package info exposes internal information about the trace-agent.
package info

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"expvar" // automatically publish `/debug/vars` on HTTP port
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

var (
	once sync.Once
	// Unfortunately there must be a global Info tracker as we publish info via expvar
	// and calls to expvar.Publish must only happen once per key
	ift = &tracker{
		receiverStats:         []TagStats{},
		languages:             nil,
		traceWriterInfo:       nil,
		statsWriterInfo:       nil,
		watchdogInfo:          watchdog.Info{},
		rateByService:         nil,
		rateByServiceFiltered: nil,
		start:                 time.Now(),
		infoTmpl:              nil,
		notRunningTmpl:        nil,
		errorTmpl:             nil,
	}
)

type tracker struct {
	infoMu        sync.RWMutex
	receiverStats []TagStats // only for the last minute
	languages     []string

	traceWriterInfo *TraceWriterInfo
	statsWriterInfo *StatsWriterInfo

	watchdogInfo  watchdog.Info
	rateByService map[string]float64
	// The rates by service with empty env values removed (As they are confusing to view for customers)
	rateByServiceFiltered map[string]float64
	start                 time.Time
	infoTmpl              *template.Template
	notRunningTmpl        *template.Template
	errorTmpl             *template.Template
}

const (
	infoTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Pid: {{.Status.Pid}}
  Uptime: {{.Status.Uptime}} seconds
  Mem alloc: {{.Status.MemStats.Alloc}} bytes

  Hostname: {{.Status.Config.Hostname}}
  Receiver: {{.Status.Config.ReceiverHost}}:{{.Status.Config.ReceiverPort}}
  Endpoints:
    {{ range $i, $e := .Status.Config.Endpoints}}
    {{ $e.Host }}
    {{end}}

  --- Receiver stats (1 min) ---

  {{ range $i, $ts := .Status.Receiver }}
  From {{if $ts.Tags.Lang}}{{ $ts.Tags.Lang }} {{ $ts.Tags.LangVersion }} ({{ $ts.Tags.Interpreter }}), client {{ $ts.Tags.TracerVersion }}{{else}}unknown clients{{end}}
    Traces received: {{ $ts.Stats.TracesReceived }} ({{ $ts.Stats.TracesBytes }} bytes)
    Spans received: {{ $ts.Stats.SpansReceived }}
    {{ with WarnString $ts }}
    WARNING: {{ . }}
    {{end}}

  {{end}}
  {{if .Status.Config.ProbabilisticSamplerEnabled}}
  Probabilistic sampling percentage: {{.Status.Config.ProbabilisticSamplerSamplingPercentage}}%
  Probabilistic sampler hash seed: {{.Status.Config.ProbabilisticSamplerHashSeed}}
  {{ else }}
  {{ range $key, $value := .Status.RateByService }}
  Priority sampling rate for '{{ $key }}': {{percent $value}} %
  {{ end }}
  {{ end }}

  --- Writer stats (1 min) ---

  Traces: {{.Status.TraceWriter.Payloads}} payloads, {{.Status.TraceWriter.Traces}} traces, {{if gt (Load .Status.TraceWriter.Events) 0}}{{Load .Status.TraceWriter.Events}} events, {{end}}{{.Status.TraceWriter.Bytes}} bytes
  {{if gt (Load .Status.TraceWriter.Errors) 0}}WARNING: Traces API errors (1 min): {{Load .Status.TraceWriter.Errors}}{{end}}
  Stats: {{Load .Status.StatsWriter.Payloads}} payloads, {{Load .Status.StatsWriter.StatsBuckets}} stats buckets, {{Load .Status.StatsWriter.Bytes}} bytes
  {{if gt (Load .Status.StatsWriter.Errors) 0}}WARNING: Stats API errors (1 min): {{Load .Status.StatsWriter.Errors}}{{end}}
`

	notRunningTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Not running or unreachable on 127.0.0.1:{{.DebugPort}}

`

	errorTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Error: {{.Error}}
  URL: {{.URL}}

`
)

// UpdateReceiverStats updates all stats associated with the Receiver
func UpdateReceiverStats(rs *ReceiverStats) {
	ift.infoMu.Lock()
	defer ift.infoMu.Unlock()
	rs.RLock()
	defer rs.RUnlock()

	s := make([]TagStats, 0, len(rs.Stats))
	for _, tagStats := range rs.Stats {
		if !tagStats.isEmpty() {
			s = append(s, *tagStats)
		}
	}

	ift.receiverStats = s
	ift.languages = rs.Languages()
}

// Languages returns all the known languages seen
func Languages() []string {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return ift.languages
}

func publishReceiverStats() interface{} {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return slices.Clone(ift.receiverStats)
}

// UpdateRateByService updates the sampling rate by service map
func UpdateRateByService(rbs map[string]float64) {
	ift.infoMu.Lock()
	defer ift.infoMu.Unlock()
	ift.rateByService = rbs

	ift.rateByServiceFiltered = make(map[string]float64, len(ift.rateByService))
	for k, v := range ift.rateByService {
		if !strings.HasSuffix(k, ",env:") {
			ift.rateByServiceFiltered[k] = v
		}
	}
}

func publishRateByService() interface{} {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return ift.rateByService
}

func publishRateByServiceFiltered() interface{} {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return ift.rateByServiceFiltered
}

// UpdateWatchdogInfo updates internal stats about the watchdog.
func UpdateWatchdogInfo(wi watchdog.Info) {
	ift.infoMu.Lock()
	defer ift.infoMu.Unlock()
	ift.watchdogInfo = wi
}

func publishWatchdogInfo() interface{} {
	ift.infoMu.RLock()
	defer ift.infoMu.RUnlock()
	return ift.watchdogInfo
}

func publishUptime() interface{} {
	return int(time.Since(ift.start) / time.Second)
}

type infoString string

func (s infoString) String() string { return string(s) }

// InitInfo initializes the info package, exposing some metrics and data via expvar
func InitInfo(conf *config.AgentConfig) error {
	var err error
	once.Do(func() {
		// Use the same error declared outside of once.Do and don't declare a new one.
		// See https://go.dev/play/p/K7sxXE2xvLp
		err = initInfo(conf, ift)
	})
	return err
}

// StatusInfo is what we use to parse expvar response.
// It does not need to contain all the fields, only those we need
// to display when called with `-info` as JSON unmarshaller will
// automatically ignore extra fields.
type StatusInfo struct {
	CmdLine  []string `json:"cmdline"`
	Pid      string   `json:"pid"`
	Uptime   int      `json:"uptime"`
	MemStats struct {
		Alloc uint64
	} `json:"memstats"`
	Version struct {
		Version   string
		GitCommit string
	} `json:"version"`
	Receiver      []TagStats         `json:"receiver"`
	RateByService map[string]float64 `json:"ratebyservice_filtered"`
	TraceWriter   TraceWriterInfo    `json:"trace_writer"`
	StatsWriter   StatsWriterInfo    `json:"stats_writer"`
	Watchdog      watchdog.Info      `json:"watchdog"`
	Config        config.AgentConfig `json:"config"`
}

func getProgramBanner(version string) (string, string) {
	program := fmt.Sprintf("Trace Agent (v %s)", version)
	banner := strings.Repeat("=", len(program))

	return program, banner
}

// Info writes a standard info message describing the running agent.
// This is not the current program, but an already running program,
// which we query with an HTTP request.
//
// If error is nil, means the program is running.
// If not, it displays a pretty-printed message anyway (for support)
func Info(w io.Writer, conf *config.AgentConfig) error {
	url := fmt.Sprintf("https://127.0.0.1:%d/debug/vars", conf.DebugServerPort)
	tr := &http.Transport{TLSClientConfig: conf.IPCTLSClientConfig}
	client := http.Client{Timeout: 3 * time.Second, Transport: tr}
	resp, err := client.Get(url)
	if err != nil {
		// OK, here, we can't even make an http call on the agent port,
		// so we can assume it's not even running, or at least, not with
		// these parameters. We display the port as a hint on where to
		// debug further, this is where the expvar JSON should come from.
		program, banner := getProgramBanner(conf.AgentVersion)
		_ = ift.notRunningTmpl.Execute(w, struct {
			Banner    string
			Program   string
			DebugPort int
		}{
			Banner:    banner,
			Program:   program,
			DebugPort: conf.DebugServerPort,
		})
		return err
	}

	defer resp.Body.Close()

	var info StatusInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		program, banner := getProgramBanner(conf.AgentVersion)
		_ = ift.errorTmpl.Execute(w, struct {
			Banner  string
			Program string
			Error   error
			URL     string
		}{
			Banner:  banner,
			Program: program,
			Error:   err,
			URL:     url,
		})
		return err
	}

	// display the remote program version, now that we know it
	program, banner := getProgramBanner(info.Version.Version)

	var buffer bytes.Buffer
	err = ift.infoTmpl.Execute(&buffer, struct {
		Banner  string
		Program string
		Status  *StatusInfo
	}{
		Banner:  banner,
		Program: program,
		Status:  &info,
	})
	if err != nil {
		return err
	}

	cleanInfo := CleanInfoExtraLines(buffer.String())

	_, err = w.Write([]byte(cleanInfo))

	return err
}

// CleanInfoExtraLines removes empty lines from template code indentation.
// The idea is that an indented empty line (only indentation spaces) is because of code indentation,
// so we remove it.
// Real legit empty lines contain no space.
func CleanInfoExtraLines(info string) string {
	var indentedEmptyLines = regexp.MustCompile("\n( +\n)+")
	return indentedEmptyLines.ReplaceAllString(info, "\n")
}

func initInfo(conf *config.AgentConfig, ift *tracker) error {
	publishVersion := func() interface{} {
		return struct {
			Version   string
			GitCommit string
		}{
			Version:   conf.AgentVersion,
			GitCommit: conf.GitCommit,
		}
	}
	funcMap := template.FuncMap{
		"add": func(a, b int64) int64 {
			return a + b
		},
		"percent": func(v float64) string {
			return fmt.Sprintf("%02.1f", v*100)
		},
		"WarnString": func(ts *TagStats) string { return ts.WarnString() },
		"Load":       func(i atomic.Int64) int64 { return i.Load() },
	}
	expvar.NewString("pid").Set(strconv.Itoa(os.Getpid()))
	expvar.Publish("uptime", expvar.Func(publishUptime))
	expvar.Publish("version", expvar.Func(publishVersion))
	expvar.Publish("receiver", expvar.Func(publishReceiverStats))
	expvar.Publish("trace_writer", expvar.Func(publishTraceWriterInfo))
	expvar.Publish("stats_writer", expvar.Func(publishStatsWriterInfo))
	expvar.Publish("ratebyservice", expvar.Func(publishRateByService))
	expvar.Publish("ratebyservice_filtered", expvar.Func(publishRateByServiceFiltered))
	expvar.Publish("watchdog", expvar.Func(publishWatchdogInfo))

	// copy the config to ensure we don't expose sensitive data such as API keys
	c := *conf
	c.Endpoints = make([]*config.Endpoint, len(conf.Endpoints))
	for i, e := range conf.Endpoints {
		c.Endpoints[i] = &config.Endpoint{Host: e.Host, NoProxy: e.NoProxy}
	}

	// Remove the TLS configs and AuthToken to avoid exposing sensitive data
	c.IPCTLSClientConfig = &tls.Config{}
	c.IPCTLSServerConfig = &tls.Config{}
	c.AuthToken = ""

	var buf []byte
	buf, err := json.Marshal(&c)
	if err != nil {
		return err
	}

	scrubbed, err := scrubber.ScrubBytes(buf)
	if err != nil {
		return err
	}

	// We keep a static copy of the config, already marshalled and stored
	// as a plain string. This saves the hassle of rebuilding it all the time
	// and avoids race issues as the source object is never used again.
	// Config is parsed at the beginning and never changed again, anyway.
	expvar.Publish("config", infoString(string(scrubbed)))

	ift.infoTmpl, err = template.New("info").Funcs(funcMap).Parse(infoTmplSrc)
	if err != nil {
		return err
	}

	ift.notRunningTmpl, err = template.New("infoNotRunning").Parse(notRunningTmplSrc)
	if err != nil {
		return err
	}

	ift.errorTmpl, err = template.New("infoError").Parse(errorTmplSrc)
	return err
}
