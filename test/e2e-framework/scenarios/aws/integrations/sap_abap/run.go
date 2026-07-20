// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sapabap

import (
	_ "embed"
	"encoding/base64"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// SAP ABAP trial container facts (pinned by evidence):
//   - SID A4H, instance number 00 -> sapstartsrv SAPControl SOAP on port 5<00>13 = 50013.
//   - Hostname vhcala4hci is MANDATORY (the ABAP profiles are keyed on it).
//   - Non-interactive start needs -agree-to-sap-license and -skip-limits-check.
const (
	containerName  = "a4h"
	sapHostname    = "vhcala4hci"
	sapControlPort = 50013

	// sapControlURL is the SAPControl SOAP endpoint the Agent container reaches.
	// :50013 is published on the host; the Agent container talks to it via the
	// default docker bridge gateway (172.17.0.1). Overridable in-container via
	// the SAP_ABAP_SAPCONTROL_URL env var referenced by conf.yaml.
	sapControlURL = "http://172.17.0.1:50013"
)

// Host paths that back the container bind mounts. The custom check .py and its
// conf.yaml are written here on the Agent host, then mounted into the Agent
// container's checks.d / conf.d trees. No secret lands in these files; conf.yaml
// references the container env via %%env_SAP_ABAP_SAPCONTROL_URL%% etc.
const (
	hostCheckPath = "/opt/sap_abap/checks.d/sap_abap.py"
	hostConfPath  = "/opt/sap_abap/conf.d/sap_abap.d/conf.yaml"

	containerCheckPath = "/etc/datadog-agent/checks.d/sap_abap.py"
	containerConfPath  = "/etc/datadog-agent/conf.d/sap_abap.d/conf.yaml"
)

// Custom check assets, embedded so the committed files carry no secret and the
// check config is version-controlled (inline Go string constants are invalid).
//
//go:embed config/sap_abap.py
var checkSource string

//go:embed config/conf.yaml
var checkConfig string

// Run deploys the sap_abap lab: a single AWS EC2 host running the SAP ABAP
// Platform Trial container (SAPControl SOAP on :50013) plus a Datadog Agent
// container whose custom sap_abap check scrapes it.
//
// Sequence (each step gated on the previous via PulumiDependsOn):
//  1. raise kernel limits required by the embedded HANA DB,
//  2. `docker login` with Docker Hub creds (token piped via stdin, never argv),
//  3. `docker run` the ABAP trial container,
//  4. readiness gate: poll SAPControl GetProcessList until the system is up,
//  5. stage the custom check assets on the host,
//  6. deploy the Agent container mounting the assets, depending on 4+5.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.DockerHostOutputs, params *Params) error {
	host, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	manager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}
	if err := manager.Export(ctx, env.DockerOutput()); err != nil {
		return err
	}

	// The SAP ABAP container is the local workload; FakeIntake is opt-out here
	// (real E2E by default).
	env.DisableFakeIntake()

	if !params.deployAgent {
		env.DisableAgent()
		return nil
	}

	if params.dockerhubUser == "" || params.dockerhubToken == "" {
		return fmt.Errorf(
			"Docker Hub credentials are required to pull the EULA-gated SAP ABAP trial image: "+
				"export %s=<hub user> and %s=<hub access token> (on an account that ACCEPTED "+
				"SAP's terms on https://hub.docker.com/r/sapse/abap-cloud-developer-trial) before "+
				"running `dda inv aws.integrations.sap_abap.create`",
			dockerhubUserEnvVar, dockerhubTokenEnvVar,
		)
	}

	// Step 1: raise kernel limits required by the ABAP trial's embedded HANA DB.
	// Without these the container fails to start the DB (shared-memory /
	// async-IO / mmap-count limits). Idempotent; failure dumps current values.
	kernelLimits, err := host.OS.Runner().Command(
		"sap-abap-kernel-limits",
		&command.Args{
			Sudo: true,
			Create: pulumi.String(`set -eu
if ! sudo sysctl -w kernel.shmmni=32768 fs.aio-max-nr=1048576 vm.max_map_count=2147483647; then
  echo "FAILED to raise kernel limits for HANA" >&2
  sudo sysctl kernel.shmmni fs.aio-max-nr vm.max_map_count >&2 || true
  exit 1
fi
echo "raised kernel limits for HANA (shmmni/aio-max-nr/max_map_count)"
`),
		},
	)
	if err != nil {
		return err
	}

	// Step 2: docker login. The token is piped via stdin from the command
	// Environment (--password-stdin); it never appears on argv or in logs.
	dockerLogin, err := host.OS.Runner().Command(
		"sap-abap-docker-login",
		&command.Args{
			Sudo: true,
			Environment: pulumi.StringMap{
				"DOCKERHUB_USER":  pulumi.String(params.dockerhubUser),
				"DOCKERHUB_TOKEN": pulumi.String(params.dockerhubToken),
			},
			Create: pulumi.String(`set -eu
if ! printf '%s' "${DOCKERHUB_TOKEN}" | sudo docker login -u "${DOCKERHUB_USER}" --password-stdin >/dev/null 2>/tmp/sap_abap_login.err; then
  echo "FAILED docker login as ${DOCKERHUB_USER}" >&2
  echo "--- docker login stderr (secrets never echoed) ---" >&2
  sed 's/[A-Za-z0-9_-]\{20,\}/<redacted>/g' /tmp/sap_abap_login.err >&2 || true
  echo "Hint: the account must have ACCEPTED SAP's EULA on the image page and the token must be valid" >&2
  exit 1
fi
echo "docker login succeeded"
`),
		},
		utils.PulumiDependsOn(kernelLimits),
	)
	if err != nil {
		return err
	}

	// Step 3: run the ABAP trial container. Hostname vhcala4hci is MANDATORY.
	// --stop-timeout 3600 gives HANA/ABAP a graceful shutdown window.
	// -agree-to-sap-license automates the EULA; -skip-limits-check bypasses the
	// container's own host-resource gate (we already raised the kernel limits).
	runContainer, err := host.OS.Runner().Command(
		"sap-abap-run-container",
		&command.Args{
			Sudo: true,
			Environment: pulumi.StringMap{
				"SAP_ABAP_IMAGE": pulumi.String(params.abapImage),
			},
			Create: pulumi.String(fmt.Sprintf(`set -eu
if sudo docker inspect %[1]s >/dev/null 2>&1; then
  echo "container %[1]s already exists; (re)starting"
  sudo docker start %[1]s
else
  if ! sudo docker run -d --name %[1]s -h %[2]s --stop-timeout 3600 \
      -p %[3]d:%[3]d -p 3200:3200 -p 8443:8443 \
      "${SAP_ABAP_IMAGE}" -agree-to-sap-license -skip-limits-check; then
    echo "FAILED to start SAP ABAP trial container from ${SAP_ABAP_IMAGE}" >&2
    sudo docker logs --tail 200 %[1]s >&2 2>/dev/null || true
    exit 1
  fi
fi
echo "SAP ABAP trial container %[1]s started"
`, containerName, sapHostname, sapControlPort)),
			Triggers: pulumi.Array{pulumi.String(params.abapImage)},
		},
		utils.PulumiDependsOn(dockerLogin),
	)
	if err != nil {
		return err
	}

	// Step 4: readiness gate. Poll SAPControl GetProcessList on the host's
	// published :50013 until the system reports up: response contains processes
	// and none are RED/GRAY (GREEN/YELLOW accepted). Generous timeout (~40 min)
	// since first boot generates the ABAP load on the fly. On timeout, dump the
	// container log tail and last SAPControl response to stderr.
	//
	// GetProcessList is SOAP 1.1 doc/literal: POST to http://localhost:50013/
	// with SOAPAction: "" and the urn:SAPControl namespace. Unauthenticated
	// read is usually allowed; HTTP 401 means read methods are protected (needs
	// a4hadm Basic auth or unprotecting via service/protectedwebmethods).
	sapReady, err := host.OS.Runner().Command(
		"sap-abap-sapcontrol-ready",
		&command.Args{
			Create: pulumi.String(fmt.Sprintf(`set -u
url="http://localhost:%d/"
soap='<?xml version="1.0" encoding="UTF-8"?><SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:urn="urn:SAPControl"><SOAP-ENV:Body><urn:GetProcessList/></SOAP-ENV:Body></SOAP-ENV:Envelope>'
for attempt in $(seq 1 120); do
  code=$(curl -sS -o /tmp/sap_abap_ready.out -w '%%{http_code}' \
    -X POST \
    -H 'Content-Type: text/xml; charset=UTF-8' \
    -H 'SOAPAction: ""' \
    --data "${soap}" \
    --max-time 20 "${url}" || echo "000")
  if [ "${code}" = "401" ]; then
    echo "attempt ${attempt}: SAPControl returned HTTP 401 (read methods protected)" >&2
    echo "Hint: unprotect GetProcessList via profile param service/protectedwebmethods, or set SAP_ABAP_SAPCONTROL_USER/PASSWORD (a4hadm)" >&2
  elif [ "${code}" = "200" ]; then
    # Up when we see at least one process and no RED/GRAY dispstatus.
    if grep -q 'SAPControl-GREEN\|SAPControl-YELLOW' /tmp/sap_abap_ready.out \
       && ! grep -q 'SAPControl-RED\|SAPControl-GRAY' /tmp/sap_abap_ready.out; then
      echo "SAP ABAP system is up (GetProcessList reports GREEN/YELLOW, no RED/GRAY)"
      exit 0
    fi
    echo "attempt ${attempt}: SAP still starting (HTTP 200, processes not all GREEN yet)" >&2
  else
    echo "attempt ${attempt}: HTTP ${code} from SAPControl" >&2
  fi
  sleep 20
done
echo "TIMED OUT waiting for SAP ABAP system to come up via SAPControl" >&2
echo "--- last SAPControl response (first 2000 bytes) ---" >&2
head -c 2000 /tmp/sap_abap_ready.out >&2 || true
echo "" >&2
echo "--- docker logs %s (tail 200) ---" >&2
sudo docker logs --tail 200 %s >&2 2>/dev/null || true
exit 1
`, sapControlPort, containerName, containerName)),
			Sudo: true,
		},
		utils.PulumiDependsOn(runContainer),
	)
	if err != nil {
		return err
	}

	// Step 5: stage the custom check .py and conf.yaml on the host. The files
	// are base64-embedded to avoid shell-quoting hazards and never contain a
	// secret (conf.yaml uses %%env_SAP_ABAP_SAPCONTROL_*%% resolved from the
	// container env). On failure it dumps diagnostics before exiting non-zero.
	checkB64 := base64.StdEncoding.EncodeToString([]byte(checkSource))
	confB64 := base64.StdEncoding.EncodeToString([]byte(checkConfig))
	writeAssets, err := host.OS.Runner().Command(
		"sap-abap-write-check-assets",
		&command.Args{
			Sudo: true,
			Create: pulumi.String(fmt.Sprintf(`set -eu
sudo install -d -m 0755 %q %q
if ! printf '%%s' '%s' | base64 -d | sudo tee %q >/dev/null; then
  echo "FAILED to write check source to %s" >&2
  sudo ls -l /opt/sap_abap/checks.d >&2 || true
  exit 1
fi
if ! printf '%%s' '%s' | base64 -d | sudo tee %q >/dev/null; then
  echo "FAILED to write check conf to %s" >&2
  sudo ls -l /opt/sap_abap/conf.d/sap_abap.d >&2 || true
  exit 1
fi
sudo chmod 0644 %q %q
echo "wrote sap_abap check assets"
sudo ls -l %q %q >&2
`,
				"/opt/sap_abap/checks.d", "/opt/sap_abap/conf.d/sap_abap.d",
				checkB64, hostCheckPath, hostCheckPath,
				confB64, hostConfPath, hostConfPath,
				hostCheckPath, hostConfPath,
				hostCheckPath, hostConfPath)),
			Triggers: pulumi.Array{pulumi.String(checkB64), pulumi.String(confB64)},
		},
		utils.PulumiDependsOn(sapReady),
	)
	if err != nil {
		return err
	}

	// Step 6: deploy the Agent container. It reaches SAPControl via the docker
	// bridge gateway (SAP_ABAP_SAPCONTROL_URL default http://172.17.0.1:50013).
	// Optional Basic-auth creds for protected read methods are injected empty by
	// default. Does not start scraping until the SAP system is up (step 4) and
	// the check assets are on the host (step 5).
	agentOptions := []dockeragentparams.Option{
		dockeragentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
		dockeragentparams.WithAgentServiceEnvVariable("SAP_ABAP_SAPCONTROL_URL", pulumi.String(sapControlURL)),
		dockeragentparams.WithAgentServiceEnvVariable("SAP_ABAP_SAPCONTROL_USER", pulumi.String(params.sapcontrolUser)),
		dockeragentparams.WithAgentServiceEnvVariable("SAP_ABAP_SAPCONTROL_PASSWORD", pulumi.String(params.sapcontrolPassword)),
		dockeragentparams.WithExtraVolumes(
			hostCheckPath+":"+containerCheckPath+":ro",
			hostConfPath+":"+containerConfPath+":ro",
		),
		dockeragentparams.WithPulumiDependsOn(
			utils.PulumiDependsOn(sapReady),
			utils.PulumiDependsOn(writeAssets),
		),
	}
	if params.agentFullImagePath != "" {
		agentOptions = append(agentOptions, dockeragentparams.WithFullImagePath(params.agentFullImagePath))
	} else if params.agentImageTag != "" {
		agentOptions = append(agentOptions, dockeragentparams.WithImageTag(params.agentImageTag))
	}

	dockerAgent, err := agent.NewDockerAgent(&awsEnv, host, manager, agentOptions...)
	if err != nil {
		return err
	}
	return dockerAgent.Export(ctx, env.DockerAgentOutput())
}

// VMRun is the no-arg pulumi.RunFunc entry point registered in the scenario registry.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	env := outputs.NewDockerHost()
	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
