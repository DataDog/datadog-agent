// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ports provides a diagnose suite for the ports used in the agent configuration
package ports

import (
	"fmt"
	"path"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/port"
)

func init() {
	diagnosis.Register("port-conflict", diagnosePortSuite)
}

var agentNames = map[string]struct{}{
	"datadog-agent": {}, "agent": {}, "trace-agent": {},
	"process-agent": {}, "system-probe": {}, "security-agent": {},
	"dogstatsd": {},
}

// diagnosePortSuite displays information about the ports used in the agent configuration
func diagnosePortSuite(_ diagnosis.Config, _ sender.DiagnoseSenderManager) []diagnosis.Diagnosis {
	ports, err := port.GetUsedPorts()
	if err != nil {
		return []diagnosis.Diagnosis{{
			Name:      "ports",
			Result:    diagnosis.DiagnosisUnexpectedError,
			Diagnosis: fmt.Sprintf("Unable to get the list of used ports: %v", err),
		}}
	}

	var diagnoses []diagnosis.Diagnosis
	for _, key := range config.Datadog.AllKeys() {
		splitKey := strings.Split(key, ".")
		keyName := splitKey[len(splitKey)-1]
		if keyName != "port" && !strings.HasPrefix(keyName, "port_") && !strings.HasSuffix(keyName, "_port") {
			continue
		}

		value := config.Datadog.GetInt(key)
		if value <= 0 {
			continue
		}

		// TODO: the ports list should be transformed to a map or sorted to improve search performance
		var found bool
		for _, p := range ports {
			// if the port is used for several protocols, add a diagnose for each
			if p.Port != uint16(value) {
				continue
			}

			found = true

			//TODO: check process user/group
			if procName, ok := isAgentProcess(p.Process); ok {
				diagnoses = append(diagnoses, diagnosis.Diagnosis{
					Name:      key,
					Result:    diagnosis.DiagnosisSuccess,
					Diagnosis: fmt.Sprintf("Required port %d is used by '%s' process (PID=%d) for %s", value, procName, p.Pid, p.Proto),
				})
				continue
			}

			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Name:      key,
				Result:    diagnosis.DiagnosisFail,
				Diagnosis: fmt.Sprintf("Required port %d is already used by '%s' process (PID=%d) for %s.", value, p.Process, p.Pid, p.Proto),
			})
		}

		if !found {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Name:      key,
				Result:    diagnosis.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is not used", value),
			})
		}
	}

	return diagnoses
}

func isAgentProcess(processName string) (string, bool) {
	processName = path.Base(processName)
	_, ok := agentNames[processName]
	return processName, ok
}

// Diagnose displays information about the ports used on the host
func Diagnose() error {
	ports, err := port.GetUsedPorts()
	if err != nil {
		return err
	}

	fmt.Println("Ports used on the host:")
	for _, p := range ports {
		processName := p.Process
		if processName == "" {
			processName = "unknown"
		}

		fmt.Printf("%5d %5s : %s\n", p.Port, p.Proto, processName)
	}

	return nil
}
