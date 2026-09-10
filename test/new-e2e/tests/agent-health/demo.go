// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

// This file holds the demo environment definitions for the agent-health team.
// They are consumed by the standalone Cobra binary in cmd/agent-health-demo,
// which drives the e2e framework directly (no dda dependency).

const (
	// PulumiScenarioName is the scenario name registered with the e2e-framework
	// registry and dispatched by AgentHealthDemoRun.
	PulumiScenarioName = "aws/agent-health-demo"

	// DefaultStackName is the base stack name used when the user does not pass one.
	// The framework prefixes it with the local username.
	DefaultStackName = "demo-agent-health-vm"

	// SSHUser is the default login user for the provisioned host.
	SSHUser = "ubuntu"
)

// ScenarioAction is a set of shell commands that implement one direction of a
// demo scenario (for example "retrigger" or "remediate"), run over SSH on the
// provisioned host.
type ScenarioAction struct {
	// Commands run in order on the remote host.
	Commands []string
	// Message printed after the action completes.
	Message string
}

// Scenario is a demo scenario with named actions for a live host.
type Scenario struct {
	// Issue is the agent-health issue this scenario demonstrates.
	Issue string
	// Description is a short human-readable summary.
	Description string
	// Actions are the named actions available once the env is running.
	Actions map[string]ScenarioAction
	// CreateDefaults are extra Pulumi config keys applied at create time.
	CreateDefaults map[string]string
}

// DemoScenarios maps scenario name to its definition for the agent-health "vm"
// demo environment. Keys match the --scenario values accepted by the CLI.
var DemoScenarios = map[string]Scenario{
	"docker-permissions": {
		Issue:       "docker_socket_permission",
		Description: "Remove dd-agent from the docker group to trigger the Docker socket permission issue",
		Actions: map[string]ScenarioAction{
			"retrigger": {
				// Inverse of the fix: drop dd-agent from the docker group so it can
				// no longer connect to the socket.
				Commands: []string{
					"sudo gpasswd -d dd-agent docker",
					"sudo systemctl restart datadog-agent",
				},
				Message: "dd-agent removed from the docker group — agent can no longer connect to the Docker socket.",
			},
			"remediate": {
				// Mirrors comp/healthplatform/issues/dockerpermissions/fix-docker-socket-permissions.sh (#55880):
				// grant dd-agent access by adding it to the docker group, then restart.
				Commands: []string{
					"sudo usermod -aG docker dd-agent",
					"sudo systemctl restart datadog-agent",
				},
				Message: "dd-agent added to the docker group and agent restarted — issue cleared.",
			},
		},
		CreateDefaults: map[string]string{},
	},
	"invalid-config": {
		Issue:       "invalid-config",
		Description: "Inject a type-mismatched value into datadog.yaml to trigger schema validation failure",
		Actions: map[string]ScenarioAction{
			"retrigger": {
				Commands: []string{
					// Remove any existing check_runners first so re-running (or running
					// right after create, which already sets it) never appends a duplicate
					// YAML key — that would break config parsing instead of producing the
					// intended schema-validation issue.
					"sudo sed -i '/^check_runners:/d' /etc/datadog-agent/datadog.yaml",
					"echo 'check_runners: \"not_a_number_for_demo\"' | sudo tee -a /etc/datadog-agent/datadog.yaml > /dev/null",
					"sudo systemctl restart datadog-agent",
				},
				Message: "Invalid config value injected — invalid-config issue will appear after the agent validates its configuration.",
			},
			"remediate": {
				Commands: []string{
					"sudo sed -i '/^check_runners:/d' /etc/datadog-agent/datadog.yaml",
					"sudo systemctl restart datadog-agent",
				},
				Message: "Invalid config value removed — issue cleared.",
			},
		},
		CreateDefaults: map[string]string{},
	},
}
