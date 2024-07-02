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

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/port"
)

var agentNames = map[string]struct{}{
	"datadog-agent": {}, "agent": {}, "trace-agent": {},
	"process-agent": {}, "system-probe": {}, "security-agent": {},
	"dogstatsd": {},
}

// DiagnosePortSuite displays information about the ports used in the agent configuration
func DiagnosePortSuite() []diagnosis.Diagnosis {
	ports, err := port.GetUsedPorts()
	if err != nil {
		return []diagnosis.Diagnosis{{
			Name:      "ports",
			Result:    diagnosis.DiagnosisUnexpectedError,
			Diagnosis: fmt.Sprintf("Unable to get the list of used ports: %v", err),
		}}
	}

	portMap := make(map[uint16]port.Port)
	for _, port := range ports {
		portMap[port.Port] = port
	}

	var diagnoses []diagnosis.Diagnosis
	for _, key := range config.Datadog().AllKeysLowercased() {
		splitKey := strings.Split(key, ".")
		keyName := splitKey[len(splitKey)-1]
		if keyName != "port" && !strings.HasPrefix(keyName, "port_") && !strings.HasSuffix(keyName, "_port") {
			continue
		}

		value := config.Datadog().GetInt(key)
		if value <= 0 {
			continue
		}

		port, ok := portMap[uint16(value)]
		// if the port is used for several protocols, add a diagnose for each
		if !ok {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Name:      key,
				Result:    diagnosis.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is not used", value),
			})
			continue
		}

		// TODO: check process user/group
		if processName, ok := isAgentProcess(port.Process); ok {
			diagnoses = append(diagnoses, diagnosis.Diagnosis{
				Name:      key,
				Result:    diagnosis.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is used by '%s' process (PID=%d) for %s", value, processName, port.Pid, port.Proto),
			})
			continue
		}

		diagnoses = append(diagnoses, diagnosis.Diagnosis{
			Name:      key,
			Result:    diagnosis.DiagnosisFail,
			Diagnosis: fmt.Sprintf("Required port %d is already used by '%s' process (PID=%d) for %s.", value, port.Process, port.Pid, port.Proto),
		})
	}

	return diagnoses
}

func isAgentProcess(processName string) (string, bool) {
	processName = path.Base(processName)
	_, ok := agentNames[processName]
	return processName, ok
}
