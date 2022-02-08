// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	ddstatus "github.com/DataDog/datadog-agent/pkg/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var httpClient = util.GetClient(false)

const (
	statusTemplate = `
==============================
Process Agent ({{ .Core.AgentVersion }})
==============================

 Status date: {{ formatUnixTime .Date }}
 Process Agent Start: {{ formatUnixTime .Expvars.UptimeNano }}
 Pid: {{ .Expvars.Pid }}
 Go Version: {{ .Core.GoVersion }}
 Python Version: {{ .Core.PythonVersion }}
 Build arch: {{ .Core.Arch }}
 Log Level: {{ .Core.Config.LogLevel }}
 Enabled Checks: {{ .Expvars.EnabledChecks }}
 Allocated Memory: {{ .Expvars.MemStats.Alloc }} bytes
 Hostname: {{ .Core.Metadata.Meta.Hostname }}

=========
Collector
=========

  Last collection time: {{.Expvars.LastCollectTime}}
  Docker socket: {{.Expvars.DockerSocket}}
  Number of processes: {{.Expvars.ProcessCount}}
  Number of containers: {{.Expvars.ContainerCount}}
  Process Queue length: {{.Expvars.ProcessQueueSize}}
  RTProcess Queue length: {{.Expvars.RTProcessQueueSize}}
  Pod Queue length: {{.Expvars.PodQueueSize}}
  Process Bytes enqueued: {{.Expvars.ProcessQueueBytes}}
  RTProcess Bytes enqueued: {{.Expvars.RTProcessQueueBytes}}
  Pod Bytes enqueued: {{.Expvars.PodQueueBytes}}

`
	notRunning = `
=============
Process Agent
=============

  The Process Agent is not running

`
)

type coreStatus struct {
	AgentVersion  string `json:"version"`
	GoVersion     string `json:"go_version"`
	PythonVersion string `json:"python_version"`
	Arch          string `json:"build_arch"`
	Config        struct {
		LogLevel string `json:"log_level"`
	} `json:"config"`
	Metadata  host.Payload        `json:"metadata"`
	Endpoints map[string][]string `json:"endpointsInfos"`
}

type infoVersion struct {
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
	GoVersion string
}

type processExpvars struct {
	Pid                 int                    `json:"pid"`
	Uptime              int                    `json:"uptime"`
	UptimeNano          float64                `json:"uptime_nano"`
	MemStats            struct{ Alloc uint64 } `json:"memstats"`
	Version             infoVersion            `json:"version"`
	DockerSocket        string                 `json:"docker_socket"`
	LastCollectTime     string                 `json:"last_collect_time"`
	ProcessCount        int                    `json:"process_count"`
	ContainerCount      int                    `json:"container_count"`
	ProcessQueueSize    int                    `json:"process_queue_size"`
	RTProcessQueueSize  int                    `json:"rtprocess_queue_size"`
	PodQueueSize        int                    `json:"pod_queue_size"`
	ProcessQueueBytes   int                    `json:"process_queue_bytes"`
	RTProcessQueueBytes int                    `json:"rtprocess_queue_bytes"`
	PodQueueBytes       int                    `json:"pod_queue_bytes"`
	ContainerID         string                 `json:"container_id"`
	ProxyURL            string                 `json:"proxy_url"`
	LogFile             string                 `json:"log_file"`
	EnabledChecks       []string               `json:"enabled_checks"`
}

type status struct {
	Date    float64
	Core    coreStatus     // Contains the status from the core agent
	Expvars processExpvars // Contains the expvars retrieved from the process agent
}

type statusOption func(s *status)

func overrideTime(t time.Time) statusOption {
	return func(s *status) {
		s.Date = float64(t.UnixNano())
	}
}

func getCoreStatus() (s coreStatus, err error) {
	addressPort, err := api.GetAPIAddressPort()
	if err != nil {
		return
	}

	statusEndpoint := fmt.Sprintf("http://%s/agent/status", addressPort)
	b, err := util.DoGet(httpClient, statusEndpoint)
	if err != nil {
		return
	}

	err = json.Unmarshal(b, &s)
	return
}

func getExpvars() (s processExpvars, err error) {
	ipcAddr, err := ddconfig.GetIPCAddress()
	if err != nil {
		return
	}

	expvarEndpoint := fmt.Sprintf("http://%s:%d/debug/vars", ipcAddr, ddconfig.Datadog.GetInt("process_config.expvar_port"))
	b, err := util.DoGet(httpClient, expvarEndpoint)
	if err != nil {
		return
	}

	err = json.Unmarshal(b, &s)
	return
}

func getStatus() (status, error) {
	coreStatus, err := getCoreStatus()
	if err != nil {
		return status{}, err
	}

	processStatus, err := getExpvars()
	if err != nil {
		return status{}, err
	}

	s := status{
		Date:    float64(time.Now().UnixNano()),
		Core:    coreStatus,
		Expvars: processStatus,
	}
	return s, nil
}

func writeNotRunning(w io.Writer) {
	_, err := fmt.Fprint(w, notRunning)
	if err != nil {
		_ = log.Error(err)
	}
}

// getAndWriteStatus calls the status server and writes it to `w`
func getAndWriteStatus(w io.Writer, options ...statusOption) {
	status, err := getStatus()
	if err != nil {
		writeNotRunning(w)
		return
	}
	for _, option := range options {
		option(&status)
	}

	tpl, err := template.New("").Funcs(ddstatus.Textfmap()).Parse(statusTemplate)
	if err != nil {
		_ = log.Error(err)
	}

	err = tpl.Execute(w, status)
	if err != nil {
		_ = log.Error(err)
	}
}

// StatusCmd returns a cobra command that prints the current status
func StatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, _ []string) error {
	err := config.LoadConfigIfExists(cmd.Flag("config").Value.String())
	if err != nil {
		return err
	}

	getAndWriteStatus(os.Stdout)
	return nil
}
