// +build windows

package config

import (
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress = "localhost:3333"
)

var (
	defaultLogFilePath = "c:\\programdata\\datadog\\logs\\process-agent.log"

	// Agent 6
	defaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
)

func init() {
	if pd, err := winutil.GetProgramDataDir(); err == nil {
		defaultLogFilePath = filepath.Join(pd, "logs", "process-agent.log")
	}
	if _here, err := executable.Folder(); err == nil {
		agentFilePath := filepath.Join(_here, "..", "..", "embedded", "agent.exe")
		if _, err := os.Stat(agentFilePath); err == nil {
			defaultDDAgentBin = agentFilePath
		}
	}
}
