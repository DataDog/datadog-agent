package runtimecfg

import (
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"gopkg.in/yaml.v3"
	"testing"
	"time"
)

var cfgClient *ProcessAgentRuntimeConfigClient

func init() {
	// Set up the logger, so we can test using log_level
	cfg, err := config.NewAgentConfig("", "", "")
	if err != nil {
		panic(err)
	}
	if err = cfg.LoadProcessYamlConfig(""); err != nil {
		panic(err)
	}

	const rpcPort = "1234"

	// Start the RPC Server
	if err := StartRuntimeSettingRPCService(rpcPort); err != nil {
		panic(err)
	}

	// Give the RPC Server a second to start up
	time.Sleep(time.Second)

	// Create a new client
	if cfgClient, err = NewProcessAgentRuntimeConfigClient(rpcPort); err != nil {
		panic(err)
	}
}

func TestGet(t *testing.T) {
	result, err := cfgClient.Get("log_level")
	if err != nil {
		t.Fatal(err)
	}

	if result != "info" {
		t.Fatal("Expected result to be 'info', got", result)
	}
}

func TestSet(t *testing.T) {
	_, err := cfgClient.Set("log_level", "debug")
	if err != nil {
		t.Fatal(err)
	}

	newSetting, err := cfgClient.Get("log_level")
	if err != nil {
		t.Fatal(err)
	}

	if newSetting != "debug" {
		t.Fatal("Expected result to be 'debug', got", newSetting)
	}
}

func TestList(t *testing.T) {
	cfgMap, err := cfgClient.List()
	if err != nil {
		t.Fatal(err)
	}

	// Since the response doesn't return actual setting.RuntimeSetting objects, we should just check the lengths
	if len(cfgMap) != len(processRuntimeSettings) {
		t.Fatal("Expected the result from client.List to be the same length as the processRuntimeSettings," +
			" but they were different.")
	}
}

func TestFullConfig(t *testing.T) {
	fullConfig, ok := ddconfig.Datadog.AllSettings()["process_config"]
	if !ok {
		t.Fatal("Could not get process_config settings")
	}
	marshal, err := yaml.Marshal(fullConfig)
	if err != nil {
		t.Fatal(err)
	}
	fullConfig = string(marshal)
	response, err := cfgClient.FullConfig()
	if err != nil {
		t.Fatal(err)
	}

	if fullConfig != response {
		t.Fatalf("Expected result to be '%s', got %s", fullConfig, response)
	}

}
