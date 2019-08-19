package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/config"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Prints the config used by a running trace-agent",
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load(agent.ConfigPath)
		if err != nil {
			return err
		}
		addr := fmt.Sprintf("http://%s:%d/debug/vars", cfg.ReceiverHost, cfg.ReceiverPort)
		resp, err := http.Get(addr)
		if err != nil {
			return fmt.Errorf("could not reach agent: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var all map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&all); err != nil {
			return err
		}
		v, err := yaml.Marshal(all["config"])
		if err != nil {
			return err
		}
		fmt.Println(string(v))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
