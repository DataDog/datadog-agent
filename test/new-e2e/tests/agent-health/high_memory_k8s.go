// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenthealth

import (
	_ "embed"
	"encoding/base64"
)

// This file holds what the high memory k8s E2E test and demo scenario share: the Helm values that give the node
// Agent a memory limit, and the commands that push its container close to that limit and release it.

//go:embed fixtures/high_memory_k8s_helm_values.yaml
var highMemoryK8sHelmValues string

// memHogScript allocates memory inside the Agent container until its working set (what the kubelet reports,
// usage minus inactive page cache) reaches 93% of the container limit, then holds it. 93% stays above the
// 90% issue threshold while leaving headroom before an OOM kill. It runs in the Agent container's cgroup,
// so the allocation is charged to the Agent container.
//
// It writes memHogPIDFile, whose path it hardcodes.
//
//go:embed fixtures/memhog.py
var memHogScript string

const (
	// highMemoryK8sScenario is the demo scenario name and demolab:demoScenario value.
	highMemoryK8sScenario = "high-memory-k8s"

	// memHogPIDFile is written by memHogScript once the target usage is reached.
	memHogPIDFile = "/tmp/memhog.pid"
	// memHogLogFile holds memHogScript output, e.g. when the container has no memory limit.
	memHogLogFile = "/tmp/memhog.log"

	// stopMemHogCommand releases the memory held by memHogScript. Run with sh -c in the Agent container.
	stopMemHogCommand = "kill $(cat " + memHogPIDFile + ") && rm -f " + memHogPIDFile

	// demoKubectl runs kubectl from the kind control plane node container, which ships kubectl and an admin
	// kubeconfig, on the agent-health demo kind VM.
	demoKubectl = `sudo docker exec "$(sudo docker ps -qf name=control-plane)" kubectl --kubeconfig /etc/kubernetes/admin.conf -n datadog`
	// demoNodeAgentPod resolves the node Agent pod name on the demo kind VM.
	demoNodeAgentPod = `"$(` + demoKubectl + ` get pod -l app.kubernetes.io/component=agent -o name | head -n1)"`
)

// startMemHogCommand starts memHogScript in the background. Run with sh -c in the Agent container. The script is
// passed base64 encoded so the command survives the SSH, docker exec and kubectl exec layers of the demo without
// any quoting.
var startMemHogCommand = "echo " + base64.StdEncoding.EncodeToString([]byte(memHogScript)) +
	" | base64 -d > /tmp/memhog.py && nohup /opt/datadog-agent/embedded/bin/python3 /tmp/memhog.py > " +
	memHogLogFile + " 2>&1 &"

// demoNodeAgentExec returns an SSH command running cmd with sh -c in the node Agent container of the demo kind VM.
// cmd must not contain single quotes.
func demoNodeAgentExec(cmd string) string {
	return demoKubectl + " exec " + demoNodeAgentPod + " -c agent -- sh -c '" + cmd + "'"
}
