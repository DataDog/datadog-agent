// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"encoding/json"
	"fmt"
	"os"
	"text/template"
	"time"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/process-agent/api"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

var httpClient = util.GetClient(false)

const (
	statusTemplate = `
==============================
Process Agent ({{ .Core.AgentVersion }})
==============================

 Status date: {{ .Date }}
 Process Agent Start: {{ .Expvars.Uptime }}
 Pid: {{ .Expvars.Pid }}
 Go Version: {{ .Core.GoVersion }}
 Python Version: {{ .Core.PythonVersion }}
 Build arch: {{ .Core.Arch }}
 Log Level: {{ .Core.Config.LogLevel }}
 Enabled Checks:

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

The process agent is not running
`
	infoErrorTmplSrc = `
=============
Process Agent
=============

	Error: {{.Error}}
`
)

func StatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE:  runStatus,
	}
}

type CoreStatus struct {
	AgentVersion  string `json:"version"`
	GoVersion     string `json:"go_version"`
	PythonVersion string `json:"python_version"`
	Arch          string `json:"build_arch"`
	Config        struct {
		LogLevel string `json:"log_level"`
	} `json:"config"`
	Metadata struct {
		Meta struct {
			Hostname string `json:"Hostname"`
		} `json:"meta"`
	} `json:"metadata"`
	Endpoints map[string][]string `json:"endpointsInfos"`
}

type InfoVersion struct {
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
	GoVersion string
}

type ProcessStatus struct {
	Pid                 int                    `json:"pid"`
	Uptime              int                    `json:"uptime"`
	MemStats            struct{ Alloc uint64 } `json:"memstats"`
	Version             InfoVersion            `json:"version"`
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
}

type status struct {
	Date    string
	Core    CoreStatus    // Contains the status from the core agent
	Expvars ProcessStatus // Contains the expvars retrieved from the process agent
}

func getCoreStatus() (s CoreStatus, err error) {
	procAddr, err := api.GetAPIAddressPort()
	if err != nil {
		return
	}

	b, err := util.DoGet(httpClient, fmt.Sprintf("http://%s/agent/status", procAddr))
	if err != nil {
		return
	}

	err = json.Unmarshal(b, &s)
	return
}

func getProcessStatus() (s ProcessStatus, err error) {
	ipcAddr, err := ddconfig.GetIPCAddress()
	if err != nil {
		return
	}

	expvarAddr := fmt.Sprintf("http://%s:%d/debug/vars", ipcAddr, ddconfig.Datadog.GetInt("process_config.expvar_port"))
	b, err := util.DoGet(httpClient, expvarAddr)
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

	processStatus, err := getProcessStatus()
	if err != nil {
		return status{}, err
	}

	return status{
		Date:    time.Now().Format(time.RFC850),
		Core:    coreStatus,
		Expvars: processStatus,
	}, nil
}

func printNotRunning() {
	fmt.Print(notRunning)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	err := config.LoadConfigIfExists(cmd.Flag("config").Value.String())
	if err != nil {
		return err
	}

	status, err := getStatus()
	if err != nil {
		printNotRunning()
	}

	tpl, err := template.New("").Parse(statusTemplate)
	if err != nil {
		return err
	}

	return tpl.Execute(os.Stdout, status)
}
