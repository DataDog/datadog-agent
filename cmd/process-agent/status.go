package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the current status",
	Long:  ``,
	RunE:  runStatus,
}

type statusResponse struct {
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

func getProcessInfo() (info *StatusInfo, err error) {
	b, err := makeRequest("/debug/vars", ddconfig.Datadog.GetInt("process_config.expvar_port"))
	if err != nil {
		return nil, fmt.Errorf("failed to reach process-agent: %v", err)
	}

	err = json.Unmarshal(b, info)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from process-agent: %v", err)
	}
	return
}

func getProcessStatus() (status *statusResponse, err error) {
	b, err := makeRequest("/agent/status", ddconfig.Datadog.GetInt("process_config.cmd_port"))
	if err != nil {
		return nil, fmt.Errorf("failed to reach process-agent: %v", err)
	}

	err = json.Unmarshal(b, &status)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from process-agent: %v", err)
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

	info, err := getProcessInfo()
	if err != nil {
		return fmt.Errorf("failed to get info from process-agent: %v", err)
	}

	fmt.Println(fmtStatus(*status, *info))
	return nil
}

func fmtBanner(str string) string {
	banner := strings.Repeat("=", len(str))

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "%s\n%s\n%[1]s\n", banner, str)

	return b.String()
}

func fmtStatus(status statusResponse, info StatusInfo) string {
	var b strings.Builder

	_, _ = fmt.Fprintln(&b, fmtBanner("Process Agent  	"))

	return b.String()
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
