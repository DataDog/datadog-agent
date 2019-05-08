package info

import (
	"bytes"
	"encoding/json"
	"expvar" // automatically publish `/debug/vars` on HTTP port

	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/trace/config"
	"github.com/StackVista/stackstate-agent/pkg/trace/sampler"
	"github.com/StackVista/stackstate-agent/pkg/trace/watchdog"
)

var (
	infoMu        sync.RWMutex
	receiverStats []TagStats // only for the last minute
	languages     []string

	// TODO: move from package globals to a clean single struct

	traceWriterInfo   TraceWriterInfo
	statsWriterInfo   StatsWriterInfo
	serviceWriterInfo ServiceWriterInfo

	watchdogInfo        watchdog.Info
	samplerInfo         SamplerInfo
	prioritySamplerInfo SamplerInfo
	errorsSamplerInfo   SamplerInfo
	rateByService       map[string]float64
	preSamplerStats     sampler.PreSamplerStats
	start               = time.Now()
	once                sync.Once
	infoTmpl            *template.Template
	notRunningTmpl      *template.Template
	errorTmpl           *template.Template
)

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
    Services received: {{ $ts.Stats.ServicesReceived }} ({{ $ts.Stats.ServicesBytes }} bytes)
    {{if gt $ts.Stats.TracesDropped 0}}
    WARNING: Traces dropped: {{ $ts.Stats.TracesDropped }}
    {{end}}
    {{if gt $ts.Stats.SpansDropped 0}}
    WARNING: Spans dropped: {{ $ts.Stats.SpansDropped }}
    {{end}}

  {{end}}
  {{ range $key, $value := .Status.RateByService }}
  Priority sampling rate for '{{ $key }}': {{percent $value}} %
  {{ end }}
  {{if lt .Status.PreSampler.Rate 1.0}}
  WARNING: Pre-sampling traces: {{percent .Status.PreSampler.Rate}} %
  {{end}}
  {{if .Status.PreSampler.Error}}
  WARNING: Pre-sampler: {{.Status.PreSampler.Error}}
  {{end}}

  --- Writer stats (1 min) ---

  Traces: {{.Status.TraceWriter.Payloads}} payloads, {{.Status.TraceWriter.Traces}} traces, {{if gt .Status.TraceWriter.Events 0}}{{.Status.TraceWriter.Events}} events, {{end}}{{.Status.TraceWriter.Bytes}} bytes
  {{if gt .Status.TraceWriter.Errors 0}}WARNING: Traces API errors (1 min): {{.Status.TraceWriter.Errors}}{{end}}
  Stats: {{.Status.StatsWriter.Payloads}} payloads, {{.Status.StatsWriter.StatsBuckets}} stats buckets, {{.Status.StatsWriter.Bytes}} bytes
  {{if gt .Status.StatsWriter.Errors 0}}WARNING: Stats API errors (1 min): {{.Status.StatsWriter.Errors}}{{end}}
  Services: {{.Status.ServiceWriter.Payloads}} payloads, {{.Status.ServiceWriter.Services}} services, {{.Status.ServiceWriter.Bytes}} bytes
  {{if gt .Status.ServiceWriter.Errors 0}}WARNING: Services API errors (1 min): {{.Status.ServiceWriter.Errors}}{{end}}
`

	notRunningTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Not running (port {{.ReceiverPort}})

`

	errorTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Error: {{.Error}}
  URL: {{.URL}}

`
)

// UpdateReceiverStats updates internal stats about the receiver.
func UpdateReceiverStats(rs *ReceiverStats) {
	infoMu.Lock()
	defer infoMu.Unlock()
	rs.RLock()
	defer rs.RUnlock()

	s := make([]TagStats, 0, len(rs.Stats))
	for _, tagStats := range rs.Stats {
		if !tagStats.isEmpty() {
			s = append(s, *tagStats)
		}
	}

	receiverStats = s
	languages = rs.Languages()
}

// Languages exposes languages reporting traces to the Agent.
func Languages() []string {
	infoMu.Lock()
	defer infoMu.Unlock()

	return languages
}

func publishReceiverStats() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return receiverStats
}

// UpdateSamplerInfo updates internal stats about signature sampling.
func UpdateSamplerInfo(ss SamplerInfo) {
	infoMu.Lock()
	defer infoMu.Unlock()

	samplerInfo = ss
}

func publishSamplerInfo() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return samplerInfo
}

// UpdatePrioritySamplerInfo updates internal stats about priority sampling.
func UpdatePrioritySamplerInfo(ss SamplerInfo) {
	infoMu.Lock()
	defer infoMu.Unlock()

	prioritySamplerInfo = ss
}

func publishPrioritySamplerInfo() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return prioritySamplerInfo
}

// UpdateErrorsSamplerInfo updates internal stats about error sampling.
func UpdateErrorsSamplerInfo(ss SamplerInfo) {
	infoMu.Lock()
	defer infoMu.Unlock()

	errorsSamplerInfo = ss
}

func publishErrorsSamplerInfo() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return errorsSamplerInfo
}

// UpdateRateByService updates the RateByService map.
func UpdateRateByService(rbs map[string]float64) {
	infoMu.Lock()
	defer infoMu.Unlock()
	rateByService = rbs
}

func publishRateByService() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return rateByService
}

// UpdateWatchdogInfo updates internal stats about the watchdog.
func UpdateWatchdogInfo(wi watchdog.Info) {
	infoMu.Lock()
	defer infoMu.Unlock()
	watchdogInfo = wi
}

func publishWatchdogInfo() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return watchdogInfo
}

// UpdatePreSampler updates internal stats about the pre-sampling.
func UpdatePreSampler(ss sampler.PreSamplerStats) {
	infoMu.Lock()
	defer infoMu.Unlock()
	preSamplerStats = ss
}

func publishPreSamplerStats() interface{} {
	infoMu.RLock()
	defer infoMu.RUnlock()
	return preSamplerStats
}

func publishUptime() interface{} {
	return int(time.Since(start) / time.Second)
}

type infoString string

func (s infoString) String() string { return string(s) }

// InitInfo initializes the info structure. It should be called only once.
func InitInfo(conf *config.AgentConfig) error {
	var err error

	funcMap := template.FuncMap{
		"add": func(a, b int64) int64 {
			return a + b
		},
		"percent": func(v float64) string {
			return fmt.Sprintf("%02.1f", v*100)
		},
	}

	once.Do(func() {
		expvar.NewInt("pid").Set(int64(os.Getpid()))
		expvar.Publish("uptime", expvar.Func(publishUptime))
		expvar.Publish("version", expvar.Func(publishVersion))
		expvar.Publish("receiver", expvar.Func(publishReceiverStats))
		expvar.Publish("sampler", expvar.Func(publishSamplerInfo))
		expvar.Publish("trace_writer", expvar.Func(publishTraceWriterInfo))
		expvar.Publish("stats_writer", expvar.Func(publishStatsWriterInfo))
		expvar.Publish("service_writer", expvar.Func(publishServiceWriterInfo))
		expvar.Publish("prioritysampler", expvar.Func(publishPrioritySamplerInfo))
		expvar.Publish("errorssampler", expvar.Func(publishErrorsSamplerInfo))
		expvar.Publish("ratebyservice", expvar.Func(publishRateByService))
		expvar.Publish("watchdog", expvar.Func(publishWatchdogInfo))
		expvar.Publish("presampler", expvar.Func(publishPreSamplerStats))

		// copy the config to ensure we don't expose sensitive data such as API keys
		c := *conf
		c.Endpoints = make([]*config.Endpoint, len(conf.Endpoints))
		for i, e := range conf.Endpoints {
			c.Endpoints[i] = &config.Endpoint{Host: e.Host, NoProxy: e.NoProxy}
		}

		var buf []byte
		buf, err = json.Marshal(&c)
		if err != nil {
			return
		}

		// We keep a static copy of the config, already marshalled and stored
		// as a plain string. This saves the hassle of rebuilding it all the time
		// and avoids race issues as the source object is never used again.
		// Config is parsed at the beginning and never changed again, anyway.
		expvar.Publish("config", infoString(string(buf)))

		infoTmpl, err = template.New("info").Funcs(funcMap).Parse(infoTmplSrc)
		if err != nil {
			return
		}

		notRunningTmpl, err = template.New("infoNotRunning").Parse(notRunningTmplSrc)
		if err != nil {
			return
		}

		errorTmpl, err = template.New("infoError").Parse(errorTmplSrc)
		if err != nil {
			return
		}
	})

	return err
}

// StatusInfo is what we use to parse expvar response.
// It does not need to contain all the fields, only those we need
// to display when called with `-info` as JSON unmarshaller will
// automatically ignore extra fields.
type StatusInfo struct {
	CmdLine  []string `json:"cmdline"`
	Pid      int      `json:"pid"`
	Uptime   int      `json:"uptime"`
	MemStats struct {
		Alloc uint64
	} `json:"memstats"`
	Version       infoVersion             `json:"version"`
	Receiver      []TagStats              `json:"receiver"`
	RateByService map[string]float64      `json:"ratebyservice"`
	TraceWriter   TraceWriterInfo         `json:"trace_writer"`
	StatsWriter   StatsWriterInfo         `json:"stats_writer"`
	ServiceWriter ServiceWriterInfo       `json:"service_writer"`
	Watchdog      watchdog.Info           `json:"watchdog"`
	PreSampler    sampler.PreSamplerStats `json:"presampler"`
	Config        config.AgentConfig      `json:"config"`
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
	url := fmt.Sprintf("http://%s:%d/debug/vars", conf.ReceiverHost, conf.ReceiverPort)
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// OK, here, we can't even make an http call on the agent port,
		// so we can assume it's not even running, or at least, not with
		// these parameters. We display the port as a hint on where to
		// debug further, this is where the expvar JSON should come from.
		program, banner := getProgramBanner(Version)
		_ = notRunningTmpl.Execute(w, struct {
			Banner       string
			Program      string
			ReceiverPort int
		}{
			Banner:       banner,
			Program:      program,
			ReceiverPort: conf.ReceiverPort,
		})
		return err
	}

	defer resp.Body.Close() // OK to defer, this is not on hot path

	var info StatusInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		program, banner := getProgramBanner(Version)
		_ = errorTmpl.Execute(w, struct {
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

	// remove the default service and env, it can be inferred from other
	// values so has little added-value and could be confusing for users.
	// Besides, if one still really wants it:
	// curl http://localhost:8126/debug/vars would show it.
	if info.RateByService != nil {
		delete(info.RateByService, "service:,env:")
	}

	var buffer bytes.Buffer

	err = infoTmpl.Execute(&buffer, struct {
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

	w.Write([]byte(cleanInfo))
	// w.Write(buffer.Bytes())

	return nil
}

// CleanInfoExtraLines removes empty lines from template code indentation.
// The idea is that an indented empty line (only indentation spaces) is because of code indentation,
// so we remove it.
// Real legit empty lines contain no space.
func CleanInfoExtraLines(info string) string {
	var indentedEmptyLines = regexp.MustCompile("\n( +\n)+")
	return indentedEmptyLines.ReplaceAllString(info, "\n")
}
