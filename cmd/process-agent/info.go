package main

import (
	"bufio"
	"encoding/json"
	"expvar"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var (
	infoMutex           sync.RWMutex
	infoOnce            sync.Once
	infoStart           = time.Now()
	infoNotRunningTmpl  *template.Template
	infoTmpl            *template.Template
	infoErrorTmpl       *template.Template
	infoDockerSocket    string
	infoLastCollectTime string
	infoProcCount       int
	infoContainerCount  int
	infoQueueSize       int
)

const (
	infoTmplSrc = `{{.Banner}}
{{.Program}}
{{.Banner}}

  Pid: {{.Status.Pid}}
  Hostname: {{.Status.Config.HostName}}
  Uptime: {{.Status.Uptime}} seconds
  Mem alloc: {{.Status.MemStats.Alloc}} bytes

  Last collection time: {{.Status.LastCollectTime}}{{if ne .Status.DockerSocket ""}}
  Docker socket: {{.Status.DockerSocket}}{{end}}
  Number of processes: {{.Status.ProcessCount}}
  Number of containers: {{.Status.ContainerCount}}
  Queue length: {{.Status.QueueSize}}

  Logs: {{.Status.Config.LogFile}}{{if .Status.ProxyURL}}
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

func publishVersion() interface{} {
	return infoVersion{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
	}
}

func publishDockerSocket() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoDockerSocket
}

func updateDockerSocket(path string) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoDockerSocket = path
}

func publishLastCollectTime() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoLastCollectTime
}

func updateLastCollectTime(t time.Time) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoLastCollectTime = t.Format("2006-01-02 15:04:05")
}

func publishProcCount() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoProcCount
}

func publishContainerCount() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoContainerCount
}

func updateProcContainerCount(msgs []model.MessageBody) {
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

	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoProcCount = procCount
	infoContainerCount = containerCount
}

func updateQueueSize(c chan checkPayload) {
	infoMutex.Lock()
	defer infoMutex.Unlock()
	infoQueueSize = len(c)
}

func publishQueueSize() interface{} {
	infoMutex.RLock()
	defer infoMutex.RUnlock()
	return infoQueueSize
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
	defer f.Close()
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

func getProgramBanner(version string) (string, string) {
	program := fmt.Sprintf("Processes and Containers Agent (v %s)", version)
	banner := strings.Repeat("=", len(program))
	return program, banner
}

type infoString string

func (s infoString) String() string { return string(s) }

type infoVersion struct {
	Version   string
	GitCommit string
	GitBranch string
	BuildDate string
	GoVersion string
}

// StatusInfo is a structure to get information from expvar and feed to template
type StatusInfo struct {
	Pid             int                    `json:"pid"`
	Uptime          int                    `json:"uptime"`
	MemStats        struct{ Alloc uint64 } `json:"memstats"`
	Version         infoVersion            `json:"version"`
	Config          config.AgentConfig     `json:"config"`
	DockerSocket    string                 `json:"docker_socket"`
	LastCollectTime string                 `json:"last_collect_time"`
	ProcessCount    int                    `json:"process_count"`
	ContainerCount  int                    `json:"container_count"`
	QueueSize       int                    `json:"queue_size"`
	ContainerID     string                 `json:"container_id"`
	ProxyURL        string                 `json:"proxy_url"`
}

func initInfo(conf *config.AgentConfig) error {
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
		expvar.NewInt("pid").Set(int64(os.Getpid()))
		expvar.Publish("uptime", expvar.Func(publishUptime))
		expvar.Publish("version", expvar.Func(publishVersion))
		expvar.Publish("docker_socket", expvar.Func(publishDockerSocket))
		expvar.Publish("last_collect_time", expvar.Func(publishLastCollectTime))
		expvar.Publish("process_count", expvar.Func(publishProcCount))
		expvar.Publish("container_count", expvar.Func(publishContainerCount))
		expvar.Publish("queue_size", expvar.Func(publishQueueSize))
		expvar.Publish("container_id", expvar.Func(publishContainerID))
		c := *conf
		var buf []byte
		buf, err = json.Marshal(&c)
		if err != nil {
			return
		}
		expvar.Publish("config", infoString(string(buf)))

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
func Info(w io.Writer, conf *config.AgentConfig, expvarURL string) error {
	var err error
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(expvarURL)
	if err != nil {
		program, banner := getProgramBanner(Version)
		_ = infoNotRunningTmpl.Execute(w, struct {
			Banner  string
			Program string
		}{
			Banner:  banner,
			Program: program,
		})
		return err
	}
	defer resp.Body.Close()

	var info StatusInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		program, banner := getProgramBanner(Version)
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

	program, banner := getProgramBanner(info.Version.Version)
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
