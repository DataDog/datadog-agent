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
		// if the port is used for several protocols, add a diagnose for each
		if !ok {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Name:      key,
				Status:    diagnose.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is not used", value),
			})
			continue
		}

		worst, sev, processName := worstBind(binds)

		// TODO: check process user/group
		if sev == severityAgent {
			diagnoses = append(diagnoses, diagnose.Diagnosis{
				Name:      key,
				Status:    diagnose.DiagnosisSuccess,
				Diagnosis: fmt.Sprintf("Required port %d is used by '%s' process (PID=%d) for %s", value, processName, worst.Pid, worst.Proto),
			})
			continue
		}

		// if the port is used by a process that is not run by the same user as the agent, we cannot retrieve the proc id
		if worst.Pid == 0 {
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
			Diagnosis: fmt.Sprintf("Required port %d is already used by '%s' process (PID=%d) for %s.", value, processName, worst.Pid, worst.Proto),
		})
	}

	return diagnoses
}

const (
	severityAgent   = 0 // agent-owned bind
	severityUnknown = 1 // Pid == 0 (different user, can't retrieve process)
	severityForeign = 2 // non-agent process with known PID — real conflict
)

// bindSeverity ranks a bind by how bad it is for the agent and returns the
// process name when known. Returning both avoids a second isAgentProcess
// lookup on the winning bind — on Windows that's a redundant syscall.
func bindSeverity(p port.Port) (int, string) {
	if p.Pid == 0 {
		return severityUnknown, ""
	}
	name, ok := isAgentProcess(p.Pid, p.Process)
	if ok {
		return severityAgent, name
	}
	return severityForeign, name
}

// worstBind picks the bind with the highest severity. A foreign conflict
// on one IP must not be masked by an agent bind on another.
func worstBind(binds []port.Port) (port.Port, int, string) {
	worst := binds[0]
	sev, name := bindSeverity(worst)
	for _, b := range binds[1:] {
		s, n := bindSeverity(b)
		if s > sev {
			worst, sev, name = b, s, n
		}
	}
	return worst, sev, name
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
