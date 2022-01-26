package main

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/spf13/cobra"
	"html/template"
	"strings"
	"time"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the current status",
	Long:  ``,
	RunE:  runStatus,
}

type StatusResponse struct {
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

func getProcessInfo(expvarPort int) (info StatusInfo, err error) {
	b, err := makeRequest("/debug/vars", expvarPort)
	if err != nil {
		return StatusInfo{}, fmt.Errorf("failed to reach process-agent: %v", err)
	}

	err = json.Unmarshal(b, &info)
	if err != nil {
		return StatusInfo{}, fmt.Errorf("failed to unmarshal response from process-agent: %v", err)
	}
	return
}

func getProcessStatus() (status StatusResponse, err error) {
	b, err := makeRequest("/agent/status", ddconfig.Datadog.GetInt("process_config.cmd_port"))
	if err != nil {
		return StatusResponse{}, fmt.Errorf("failed to reach process-agent: %v", err)
	}

	err = json.Unmarshal(b, &status)
	if err != nil {
		return StatusResponse{}, fmt.Errorf("failed to unmarshal response from process-agent: %v", err)
	}
	return
}

func runStatus(cmd *cobra.Command, args []string) error {
	// Set up the config so we can get the port later
	cfg := config.NewDefaultAgentConfig(false)
	if opts.configPath != "" {
		if err := config.LoadConfigIfExists(opts.configPath); err != nil {
			return err
		}
	}
	err := cfg.LoadProcessYamlConfig(opts.configPath)
	if err != nil {
		return err
	}

	status, err := getProcessStatus()
	if err != nil {
		return fmt.Errorf("failed to get status from process-agent: %v", err)
	}

	info, err := getProcessInfo(cfg.ProcessExpVarPort)
	if err != nil {
		return fmt.Errorf("failed to get info from process-agent: %v", err)
	}

	printStatus(status, info)
	return nil
}

func printStatus(status StatusResponse, info StatusInfo) {
	files := template.Must(template.ParseFiles("./status.tmpl"))

	fmt.Printf("%#v\n\n%#v\n", status, info)
	var b strings.Builder
	err := files.Execute(&b, struct {
		StatusDate string
		StatusResponse
		StatusInfo
	}{
		StatusDate:     time.Now().Format(time.UnixDate),
		StatusResponse: status,
		StatusInfo:     info,
	})
	if err != nil {
		_ = log.Warn(err)
	}
	fmt.Println(b.String())
}

func makeRequest(path string, port int) ([]byte, error) {
	c := util.GetClient(false)

	ipcAddress, err := ddconfig.GetIPCAddress()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("http://%v:%v%s", ipcAddress, port, path)

	r, err := util.DoGet(c, url)
	if err != nil {
		return nil, fmt.Errorf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues. \n", err)
	}

	return r, nil
}
