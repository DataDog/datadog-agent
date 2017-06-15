package embed

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
	yaml "gopkg.in/yaml.v2"
)

const (
	windowsToken       = '\\'
	unixToken          = '/'
	autoDiscoveryToken = "#### AUTO-DISCOVERY ####\n"
)

// JMXCheckLoader is a specific loader for checks living in this package
type JMXCheckLoader struct {
	ipc util.NamedPipe
}

// NewJMXCheckLoader creates a loader for go checks
func NewJMXCheckLoader() *JMXCheckLoader {
	basePath := config.Datadog.GetString("jmx_pipe_path")
	pipeName := config.Datadog.GetString("jmx_pipe_name")

	var sep byte
	var pipePath string
	if strings.Contains(basePath, string(windowsToken)) {
		sep = byte(windowsToken)
	} else {
		sep = byte(unixToken)
	}

	if basePath[len(basePath)-1] == sep {
		pipePath = fmt.Sprintf("%s%s", basePath, pipeName)
	} else {
		pipePath = fmt.Sprintf("%s%c%s", basePath, sep, pipeName)
	}

	pipe, err := util.GetPipe(pipePath)
	if err != nil {
		log.Errorf("Error getting pipe: %v", err)
		return nil
	}

	if err := pipe.Open(); err != nil {
		log.Errorf("Error opening pipe: %v", err)
		return nil
	}

	return &JMXCheckLoader{ipc: pipe}
}

// Load returns an (empty?) list of checks and nil if it all works out
func (jl *JMXCheckLoader) Load(config check.Config) ([]check.Check, error) {
	var err error
	checks := []check.Check{}

	if !jl.ipc.Ready() {
		return checks, errors.New("pipe unavailable - cannot load check configuration")
	}

	isJMX := false
	for _, check := range jmxChecks {
		if check == config.Name {
			isJMX = true
			break
		}
	}

	if !isJMX {
		// Unmarshal initConfig to a RawConfigMap
		rawInitConfig := check.ConfigRawMap{}
		err = yaml.Unmarshal(config.InitConfig, &rawInitConfig)
		if err != nil {
			log.Errorf("error in yaml %s", err)
			return checks, err
		}

		x, ok := rawInitConfig["is_jmx"]
		if !ok {
			return checks, errors.New("not a JMX check")
		}

		isJMX, ok := x.(bool)
		if !isJMX || !ok {
			return checks, errors.New("unable to determine if check is JMX compatible")
		}
	}

	var yamlBuff bytes.Buffer
	yamlBuff.Write([]byte(fmt.Sprintf("%s\n", autoDiscoveryToken)))
	yamlBuff.Write([]byte(fmt.Sprintf("# %s_0\n", config.Name)))
	yamlBuff.Write([]byte(config.String()))

	_, err = jl.ipc.Write([]byte(yamlBuff.String()))

	return checks, err
}

func (jl *JMXCheckLoader) String() string {
	return "JMX Check Loader"
}
