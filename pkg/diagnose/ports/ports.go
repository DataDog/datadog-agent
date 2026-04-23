// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ports provides a diagnose suite for the ports used in the agent configuration
package ports

import (
	"fmt"
	"strings"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/port"
)

var agentNames = map[string]struct{}{
	"agent": {}, "trace-agent": {}, "trace-loader": {}, "process-agent": {},
	"system-probe": {}, "security-agent": {}, "agent-data-plane": {},
	"privateactionrunner": {},
}

// DiagnosePortSuite displays information about the ports used in the agent configuration
func DiagnosePortSuite() []diagnose.Diagnosis {
	ports, err := port.GetUsedPorts()
	if err != nil {
		return []diagnose.Diagnosis{{
			Name:      "ports",
			Status:    diagnose.DiagnosisUnexpectedError,
			Diagnosis: fmt.Sprintf("Unable to get the list of used ports: %v", err),
		}}
	}

	portMap := make(map[uint16][]port.Port)
	for _, p := range ports {
		portMap[p.Port] = append(portMap[p.Port], p)
	}

	var diagnoses []diagnose.Diagnosis
	for _, key := range pkgconfigsetup.Datadog().AllKeysLowercased() {
		splitKey := strings.Split(key, ".")
		keyName := splitKey[len(splitKey)-1]
		if keyName != "port" && !strings.HasPrefix(keyName, "port_") && !strings.HasSuffix(keyName, "_port") {
			continue
		}

		value := pkgconfigsetup.Datadog().GetInt(key)
		if value <= 0 {
			continue
		}

		binds, ok := portMap[uint16(value)]
		if !ok {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Name:      key,
				Status:    diagnose.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is not used", value),
			})
			continue
		}

		// A port can be bound on multiple IPs; pick the worst bind so a
		// foreign conflict on one IP isn't masked by an agent bind on another.
		port := binds[0]
		sev := bindSeverity(port)
		for _, b := range binds[1:] {
			if s := bindSeverity(b); s > sev {
				port, sev = b, s
			}
		}

		// TODO: check process user/group
		processName, ok := isAgentProcess(port.Pid, port.Process)
		if ok {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Name:      key,
				Status:    diagnose.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is used by '%s' process (PID=%d) for %s", value, processName, port.Pid, port.Proto),
			})
			continue
		}

		// if the port is used by a process that is not run by the same user as the agent, we cannot retrieve the proc id
		if port.Pid == 0 {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Name:      key,
				Status:    diagnose.DiagnosisWarning,
				Diagnosis: fmt.Sprintf("Required port %d is already used by an another process. Ensure this is the expected process.", value),
			})
			continue
		}

		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Name:      key,
			Status:    diagnose.DiagnosisFail,
			Diagnosis: fmt.Sprintf("Required port %d is already used by '%s' process (PID=%d) for %s.", value, processName, port.Pid, port.Proto),
		})
	}

	return diagnoses
}

// bindSeverity ranks a bind by how bad it is for the agent:
// 2 = foreign process with known PID (real conflict),
// 1 = unknown owner (Pid == 0, different user),
// 0 = agent-owned.
func bindSeverity(p port.Port) int {
	if p.Pid == 0 {
		return 1
	}
	if _, ok := isAgentProcess(p.Pid, p.Process); ok {
		return 0
	}
	return 2
}

// isAgentProcess checks if the given pid corresponds to an agent process
func isAgentProcess(pid int, processName string) (string, bool) {
	processName, err := RetrieveProcessName(pid, processName)
	if err != nil {
		return "", false
	}
	_, ok := agentNames[processName]
	return processName, ok
}
