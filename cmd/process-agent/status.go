package main

import (
	"encoding/json"
	"fmt"
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
	Pid           int    `json:"process.Pid"`
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

func runStatus(cmd *cobra.Command, args []string) error {
	// Set up the config so we can get the port later
	// We set this up differently from the main process-agent because this way is quieter
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

	ipcAddress, err := ddconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	statusURI := fmt.Sprintf("http://%v:%v/agent/status", ipcAddress, ddconfig.Datadog.GetInt("process_config.cmd_port"))
	b, err := makeRequest(statusURI)
	if err != nil {
		return fmt.Errorf("failed to reach process-agent: %v", err)
	}

	fmt.Println("Response from agent")
	fmt.Printf("%s\n\n", b)

	var status statusResponse
	err = json.Unmarshal(b, &status)
	if err != nil {
		return fmt.Errorf("failed to unmarshal response from process-agent: %v", err)
	}

	fmt.Println("output:")
	b, _ = json.MarshalIndent(status, "", "  ")
	fmt.Printf("%s\n", b)
	return nil
}

func makeRequest(url string) ([]byte, error) {
	var e error
	c := util.GetClient(false) // FIX: get certificates right then make this true

	// Set session token
	//e = util.SetAuthToken()
	//if e != nil {
	//	return nil, e
	//}

	r, e := util.DoGet(c, url)
	if e != nil {
		var errMap = make(map[string]string)
		json.Unmarshal(r, &errMap) //nolint:errcheck
		// If the error has been marshalled into a json object, check it and return it properly
		if err, found := errMap["error"]; found {
			e = fmt.Errorf(err)
		}

		fmt.Printf("Could not reach agent: %v \nMake sure the agent is running before requesting the status and contact support if you continue having issues. \n", e)
		return nil, e
	}

	return r, nil

}
