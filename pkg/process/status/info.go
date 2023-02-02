// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"bufio"
	"encoding/json"
	"expvar"
	"fmt"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	model "github.com/DataDog/agent-payload/v5/process"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	infoMutex                 sync.RWMutex
	infoOnce                  sync.Once
	infoStart                 = time.Now()
	infoNotRunningTmpl        *template.Template
	infoTmpl                  *template.Template
	infoErrorTmpl             *template.Template
	infoDockerSocket          string
	infoLastCollectTime       string
	infoProcCount             atomic.Int64
	infoContainerCount        atomic.Int64
	infoProcessQueueSize      atomic.Int64
	infoRTProcessQueueSize    atomic.Int64
	infoConnectionsQueueSize  atomic.Int64
	infoEventQueueSize        atomic.Int64
	infoPodQueueSize          atomic.Int64
	infoProcessQueueBytes     atomic.Int64
	infoRTProcessQueueBytes   atomic.Int64
	infoConnectionsQueueBytes atomic.Int64
	infoEventQueueBytes       atomic.Int64
	infoPodQueueBytes         atomic.Int64
	infoEnabledChecks         []string
	infoDropCheckPayloads     []string
)

const (
	infoTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Pid: {{.Status.Pid}}
  Hostname: {{.Status.HostName}}
  Uptime: {{.Status.Uptime}} seconds
  Mem alloc: {{.Status.MemStats.Alloc}} bytes
  {{- if .Status.SystemProbeProcessModuleEnabled }}
  System Probe Process Module Status: Running
  {{- else}}
  System Probe Process Module Status: Not running
  {{- end}}

  Last collection time: {{.Status.LastCollectTime}}{{if ne .Status.DockerSocket ""}}
  Docker socket: {{.Status.DockerSocket}}{{end}}
  Number of processes: {{.Status.ProcessCount}}
  Number of containers: {{.Status.ContainerCount}}
  Process Queue length: {{.Status.ProcessQueueSize}}
  RTProcess Queue length: {{.Status.RTProcessQueueSize}}
  Connections Queue length: {{.Status.ConnectionsQueueSize}}
  Event Queue length: {{.Status.EventQueueSize}}
  Pod Queue length: {{.Status.PodQueueSize}}
  Process Bytes enqueued: {{.Status.ProcessQueueBytes}}
  RTProcess Bytes enqueued: {{.Status.RTProcessQueueBytes}}
  Connections Bytes enqueued: {{.Status.ConnectionsQueueBytes}}
  Event Bytes enqueued: {{.Status.EventQueueBytes}}
  Pod Bytes enqueued: {{.Status.PodQueueBytes}}
  Drop Check Payloads: {{.Status.DropCheckPayloads}}

  Logs: {{.Status.LogFile}}{{if .Status.ProxyURL}}
  HttpProxy: {{.Status.ProxyURL}}{{end}}{{if ne .Status.ContainerID ""}}
  Container ID: {{.Status.ContainerID}}{{end}}

`
	infoNotRunningTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Not running

`
	infoErrorTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Error: {{.Error}}

`
)

func publishUptime() interface{} {
	return int(time.Since(infoStart) / time.Second)
}

func publishUptimeNano() interface{} {
	return infoStart.UnixNano()
}

func publishVersion() interface{} {
	agentVersion, _ := version.Agent()
	return agentVersion
}

func publishDockerSocket() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoDockerSocket
}

func UpdateDockerSocket(path string) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoDockerSocket = path
}

func publishLastCollectTime() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoLastCollectTime
}

func UpdateLastCollectTime(t time.Time) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoLastCollectTime = t.Format("2006-01-02 15:04:05")
}

func publishInt(i *atomic.Int64) expvar.Func {
	return func() any {
		return i.Load()
	}
}

func publishBool(v bool) expvar.Func {
	return func() any {
		return v
	}
}

func UpdateProcContainerCount(msgs []model.MessageBody) {
	var procCount, containerCount int
	for _, m := range msgs {
		switch msg := m.(type) {
		case *model.CollectorContainer:
			containerCount += len(msg.Containers)
		case *model.CollectorProc:
			procCount += len(msg.Processes)
			containerCount += len(msg.Containers)
		}
	}

	infoProcCount.Store(int64(procCount))
	infoContainerCount.Store(int64(containerCount))
}

type QueueStats struct {
	ProcessQueueSize      int
	RtProcessQueueSize    int
	ConnectionsQueueSize  int
	EventQueueSize        int
	PodQueueSize          int
	ProcessQueueBytes     int64
	RtProcessQueueBytes   int64
	ConnectionsQueueBytes int64
	EventQueueBytes       int64
	PodQueueBytes         int64
}

func UpdateQueueStats(stats *QueueStats) {
	infoProcessQueueSize.Store(int64(stats.ProcessQueueSize))
	infoRTProcessQueueSize.Store(int64(stats.RtProcessQueueSize))
	infoConnectionsQueueSize.Store(int64(stats.ConnectionsQueueSize))
	infoEventQueueSize.Store(int64(stats.EventQueueSize))
	infoPodQueueSize.Store(int64(stats.PodQueueSize))
	infoProcessQueueBytes.Store(stats.ProcessQueueBytes)
	infoRTProcessQueueBytes.Store(stats.RtProcessQueueBytes)
	infoConnectionsQueueBytes.Store(stats.ConnectionsQueueBytes)
	infoEventQueueBytes.Store(stats.EventQueueBytes)
	infoPodQueueBytes.Store(stats.PodQueueBytes)
}

func UpdateEnabledChecks(enabledChecks []string) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoEnabledChecks = enabledChecks
}

func publishEnabledChecks() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoEnabledChecks
}

func publishContainerID() interface{} {
	cgroupFile := "/proc/self/cgroup"
	if !util.PathExists(cgroupFile) {
		return nil
	}
	f, err := os.Open(cgroupFile)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	// the content of the file should have "docker/" on each line, the last
	// bit of each line after the "/" should be the container id, e.g.
	//
	// 	11:name=systemd:/docker/49de419da182a44f29659b9761a963543cdbf1dee8b51313b9104edec4461c58
	//
	// we could just extract that and treat it as current container id
	containerID := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "docker/") {
			slices := strings.Split(line, "/")
			// it's not totally safe to assume the format, but it's the only thing we can do for now
			if len(slices) == 3 {
				containerID = slices[len(slices)-1]
				break
			}
		}
	}
	return containerID
}

func publishEndpoints(eps []apicfg.Endpoint) func() interface{} {
	return func() interface{} {
		return getEndpointsInfo(eps)
	}
}

func getEndpointsInfo(eps []apicfg.Endpoint) interface{} {
	endpointsInfo := make(map[string][]string)

	// obfuscate the api keys
	for _, endpoint := range eps {
		apiKey := endpoint.APIKey
		if len(apiKey) > 5 {
			apiKey = apiKey[len(apiKey)-5:]
		}

		endpointsInfo[endpoint.Endpoint.String()] = append(endpointsInfo[endpoint.Endpoint.String()], apiKey)
	}
	return endpointsInfo
}

func UpdateDropCheckPayloads(drops []string) {
	infoMutex.RLock()
	defer infoMutex.RUnlock()

	infoDropCheckPayloads = make([]string, len(drops))
	copy(infoDropCheckPayloads, drops)
}

func publishDropCheckPayloads() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()

	return infoDropCheckPayloads
}

func getProgramBanner(version string) (string, string) {
	program := fmt.Sprintf("Processes and Containers Agent (v %s)", version)
	banner := strings.Repeat("=", len(program))
	return program, banner
}

// StatusInfo is a structure to get information from expvar and feed to template
type StatusInfo struct {
	Pid                             int                    `json:"pid"`
	HostName                        string                 `json:"host"`
	Uptime                          int                    `json:"uptime"`
	MemStats                        struct{ Alloc uint64 } `json:"memstats"`
	Version                         version.Version        `json:"version"`
	DockerSocket                    string                 `json:"docker_socket"`
	LastCollectTime                 string                 `json:"last_collect_time"`
	ProcessCount                    int                    `json:"process_count"`
	ContainerCount                  int                    `json:"container_count"`
	ProcessQueueSize                int                    `json:"process_queue_size"`
	RTProcessQueueSize              int                    `json:"rtprocess_queue_size"`
	ConnectionsQueueSize            int                    `json:"connections_queue_size"`
	EventQueueSize                  int                    `json:"event_queue_size"`
	PodQueueSize                    int                    `json:"pod_queue_size"`
	ProcessQueueBytes               int                    `json:"process_queue_bytes"`
	RTProcessQueueBytes             int                    `json:"rtprocess_queue_bytes"`
	ConnectionsQueueBytes           int                    `json:"connections_queue_bytes"`
	EventQueueBytes                 int                    `json:"event_queue_bytes"`
	PodQueueBytes                   int                    `json:"pod_queue_bytes"`
	ContainerID                     string                 `json:"container_id"`
	ProxyURL                        string                 `json:"proxy_url"`
	LogFile                         string                 `json:"log_file"`
	DropCheckPayloads               []string               `json:"drop_check_payloads"`
	SystemProbeProcessModuleEnabled bool                   `json:"system_probe_process_module_enabled"`
}

func InitInfo(hostname string, processModuleEnabled bool, eps []apicfg.Endpoint) error {
	var err error

	funcMap := template.FuncMap{
		"add": func(a, b int64) int64 {
			return a + b
		},
		"percent": func(v float64) string {
			return fmt.Sprintf("%02.1f", v*100)
		},
	}
	infoOnce.Do(func() {
		expvar.NewString("host").Set(hostname)
		expvar.NewInt("pid").Set(int64(os.Getpid()))
		expvar.Publish("uptime", expvar.Func(publishUptime))
		expvar.Publish("uptime_nano", expvar.Func(publishUptimeNano))
		expvar.Publish("version", expvar.Func(publishVersion))
		expvar.Publish("docker_socket", expvar.Func(publishDockerSocket))
		expvar.Publish("last_collect_time", expvar.Func(publishLastCollectTime))
		expvar.Publish("process_count", publishInt(&infoProcCount))
		expvar.Publish("container_count", publishInt(&infoContainerCount))
		expvar.Publish("process_queue_size", publishInt(&infoProcessQueueSize))
		expvar.Publish("rtprocess_queue_size", publishInt(&infoRTProcessQueueSize))
		expvar.Publish("connections_queue_size", publishInt(&infoConnectionsQueueSize))
		expvar.Publish("event_queue_size", publishInt(&infoEventQueueSize))
		expvar.Publish("pod_queue_size", publishInt(&infoPodQueueSize))
		expvar.Publish("process_queue_bytes", publishInt(&infoProcessQueueBytes))
		expvar.Publish("rtprocess_queue_bytes", publishInt(&infoRTProcessQueueBytes))
		expvar.Publish("connections_queue_bytes", publishInt(&infoConnectionsQueueBytes))
		expvar.Publish("event_queue_bytes", publishInt(&infoEventQueueBytes))
		expvar.Publish("pod_queue_bytes", publishInt(&infoPodQueueBytes))
		expvar.Publish("container_id", expvar.Func(publishContainerID))
		expvar.Publish("enabled_checks", expvar.Func(publishEnabledChecks))
		expvar.Publish("endpoints", expvar.Func(publishEndpoints(eps)))
		expvar.Publish("drop_check_payloads", expvar.Func(publishDropCheckPayloads))
		expvar.Publish("system_probe_process_module_enabled", publishBool(processModuleEnabled))

		infoTmpl, err = template.New("info").Funcs(funcMap).Parse(infoTmplSrc)
		if err != nil {
			return
		}
		infoNotRunningTmpl, err = template.New("infoNotRunning").Parse(infoNotRunningTmplSrc)
		if err != nil {
			return
		}
		infoErrorTmpl, err = template.New("infoError").Parse(infoErrorTmplSrc)
		if err != nil {
			return
		}
	})

	return err
}

// Info is called when --info flag is enabled when executing the agent binary
func Info(w io.Writer, expvarURL string) error {
	agentVersion, _ := version.Agent()
	var err error
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(expvarURL)
	if err != nil {
		program, banner := getProgramBanner(agentVersion.GetNumber())
		_ = infoNotRunningTmpl.Execute(w, struct {
			Banner  string
			Program string
		}{
			Banner:  banner,
			Program: program,
		})
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var info StatusInfo
	info.LogFile = ddconfig.Datadog.GetString("process_config.log_file")
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		// Since the request failed, we can't get the version of the remote agent.
		clientVersion, _ := version.Agent()
		program, banner := getProgramBanner(clientVersion.GetNumber())
		_ = infoErrorTmpl.Execute(w, struct {
			Banner  string
			Program string
			Error   error
		}{
			Banner:  banner,
			Program: program,
			Error:   err,
		})
		return err
	}

	program, banner := getProgramBanner(info.Version.GetNumber())
	err = infoTmpl.Execute(w, struct {
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
	return nil
}
