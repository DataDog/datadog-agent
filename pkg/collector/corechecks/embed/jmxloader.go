// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build jmx

package embed

import (
	// "bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"
)

const (
	windowsToken       = '\\'
	unixToken          = '/'
	autoDiscoveryToken = "#### AUTO-DISCOVERY ####\n"
)

// JMXCheckLoader is a specific loader for checks living in this package
type JMXCheckLoader struct {
	ipc    util.NamedPipe
	checks []string
}

// NewJMXCheckLoader creates a loader for go checks
func NewJMXCheckLoader() (*JMXCheckLoader, error) {
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
		return nil, errors.New("unable to initialize pipe")
	}

	if err := pipe.Open(); err != nil {
		log.Errorf("Error opening pipe: %v", err)
		return nil, errors.New("unable to initialize pipe")
	}

	return &JMXCheckLoader{ipc: pipe, checks: []string{}}, nil
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
		if !check.IsConfigJMX(config.InitConfig) {
			return checks, errors.New("check is not a jmx check, or unable to determine if it's so")
		}
		isJMX = true
	}

	// TODO: writing to a pipe will block - this will instead drop the config in
	//       the GRPC cache, and let JMXFetch collect the configs on-demand.
	//       Commenting out for now.
	// var yamlBuff bytes.Buffer
	// yamlBuff.Write([]byte(fmt.Sprintf("%s\n", autoDiscoveryToken)))
	// yamlBuff.Write([]byte(fmt.Sprintf("# %s_0\n", config.Name)))
	// yamlBuff.Write([]byte(config.String()))

	// _, err = jl.ipc.Write([]byte(yamlBuff.String()))
	factory := core.GetCheckFactory("jmx")
	if factory == nil {
		return checks, fmt.Errorf("check jmx not found in catalog")
	}

	launcher := factory()
	j, ok := launcher.(*JMXCheck)
	if ok {
		configured := false
		for _, instance := range config.Instances {
			if err := j.Configure(instance, config.InitConfig); err != nil {
				log.Errorf("jmx.loader: could not configure check %s: %s", j, err)
				continue
			}
			configured = true
		}

		if configured {
			j.checks[fmt.Sprintf("%s.yaml", config.Name)] = struct{}{} // exists
			checks = append(checks, j)
		} else {
			err = fmt.Errorf("No instances successfully configured.")
		}
	}

	return checks, err
}

func (jl *JMXCheckLoader) String() string {
	return "JMX Check Loader"
}

func init() {
	factory := func() (check.Loader, error) {
		return NewJMXCheckLoader()
	}

	loaders.RegisterLoader(30, factory)
}
