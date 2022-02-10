package util

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"runtime"
	"time"

	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var httpClient = apiutil.GetClient(false)

type CoreStatus struct {
	AgentVersion  string       `json:"version"`
	GoVersion     string       `json:"go_version"`
	PythonVersion string       `json:"python_version"`
	Arch          string       `json:"build_arch"`
	Config        ConfigStatus `json:"config"`
	Metadata      host.Payload `json:"metadata"`
}

type ConfigStatus struct {
	LogLevel string `json:"log_level"`
}
type InfoVersion struct {
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
	GoVersion string
}

type ProcessExpvars struct {
	Pid        int     `json:"pid"`
	Uptime     int     `json:"uptime"`
	UptimeNano float64 `json:"uptime_nano"`
	MemStats   struct {
		Alloc uint64 `json:"alloc"`
	} `json:"memstats"`
	Version             InfoVersion         `json:"version"`
	DockerSocket        string              `json:"docker_socket"`
	LastCollectTime     string              `json:"last_collect_time"`
	ProcessCount        int                 `json:"process_count"`
	ContainerCount      int                 `json:"container_count"`
	ProcessQueueSize    int                 `json:"process_queue_size"`
	RTProcessQueueSize  int                 `json:"rtprocess_queue_size"`
	PodQueueSize        int                 `json:"pod_queue_size"`
	ProcessQueueBytes   int                 `json:"process_queue_bytes"`
	RTProcessQueueBytes int                 `json:"rtprocess_queue_bytes"`
	PodQueueBytes       int                 `json:"pod_queue_bytes"`
	ContainerID         string              `json:"container_id"`
	ProxyURL            string              `json:"proxy_url"`
	LogFile             string              `json:"log_file"`
	EnabledChecks       []string            `json:"enabled_checks"`
	Endpoints           map[string][]string `json:"endpoints"`
}

type Status struct {
	Date    float64
	Core    CoreStatus     // Contains the status from the core agent
	Expvars ProcessExpvars // Contains the expvars retrieved from the process agent
}

type StatusOption func(s *Status)

type ConnectionError struct {
	error
}

func NewConnectionError(err error) ConnectionError {
	return ConnectionError{err}
}

func OverrideTime(t time.Time) StatusOption {
	return func(s *Status) {
		s.Date = float64(t.UnixNano())
	}
}

func getCoreStatus() (s CoreStatus) {
	hostnameData, err := util.GetHostnameData(context.TODO())
	var metadata *host.Payload
	if err != nil {
		log.Errorf("Error grabbing hostname for status: %v", err)
		metadata = host.GetPayloadFromCache(context.TODO(), util.HostnameData{Hostname: "unknown", Provider: "unknown"})
	} else {
		metadata = host.GetPayloadFromCache(context.TODO(), hostnameData)
	}

	return CoreStatus{
		AgentVersion:  version.AgentVersion,
		GoVersion:     runtime.Version(),
		PythonVersion: host.GetPythonVersion(),
		Arch:          runtime.GOARCH,
		Config: ConfigStatus{
			LogLevel: ddconfig.Datadog.GetString("log_level"),
		},
		Metadata: *metadata,
	}
}

func getExpvars() (s ProcessExpvars, err error) {
	ipcAddr, err := ddconfig.GetIPCAddress()
	if err != nil {
		return ProcessExpvars{}, fmt.Errorf("config error: %s", err.Error())
	}

	port := ddconfig.Datadog.GetInt("process_config.expvar_port")
	if port <= 0 {
		_ = log.Warnf("Invalid process_config.expvar_port -- %d, using default port %d\n", port, ddconfig.DefaultProcessExpVarPort)
		port = ddconfig.DefaultProcessExpVarPort
	}
	expvarEndpoint := fmt.Sprintf("http://%s:%d/debug/vars", ipcAddr, port)
	b, err := apiutil.DoGet(httpClient, expvarEndpoint)
	if err != nil {
		return s, ConnectionError{err}
	}

	err = json.Unmarshal(b, &s)
	return
}

func GetStatus() map[string]interface{} {
	stats := make(map[string]interface{})

	coreStatus := getCoreStatus()

	processStatus, err := getExpvars()
	if err != nil {
		stats["error"] = fmt.Sprintf("%v", err.Error())
		return stats
	}

	stats["date"] = float64(time.Now().UnixNano())
	stats["core"] = coreStatus
	stats["expvars"] = processStatus

	return stats
}
