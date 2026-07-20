// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sapalm

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

// sandboxURL is the SAP Cloud ALM Analytics public sandbox base URL. The custom
// sap_alm check polls this external host; the lab deploys no SAP workload.
const sandboxURL = "https://sandbox.api.sap.com/SAPCALM"

// Host paths that back the container bind mounts. The custom check .py and its
// conf.yaml are written here on the Agent host, then mounted into the Agent
// container's checks.d / conf.d trees. The APIKey never lands in these files;
// conf.yaml references the container env via %%env_SAP_ALM_API_KEY%%.
const (
	hostCheckPath = "/opt/sap_alm/checks.d/sap_alm.py"
	hostConfPath  = "/opt/sap_alm/conf.d/sap_alm.d/conf.yaml"

	containerCheckPath = "/etc/datadog-agent/checks.d/sap_alm.py"
	containerConfPath  = "/etc/datadog-agent/conf.d/sap_alm.d/conf.yaml"
)

// Custom check assets, embedded so the committed files carry no secret and the
// check config is version-controlled (inline Go string constants are invalid).
//
//go:embed config/sap_alm.py
var checkSource string

//go:embed config/conf.yaml
var checkConfig string

// Run deploys the sap_alm lab: a single Docker Datadog Agent host on AWS that
// polls the external SAP Cloud ALM Analytics sandbox. The SAP Business
// Accelerator Hub APIKey is injected into the Agent container as SAP_ALM_API_KEY.
//
// The custom check (checks.d/sap_alm.py + conf.d/sap_alm.d/conf.yaml) is
// synthesized in a later phase after live sandbox exploration; this infra
// skeleton stands up the Agent host and wires the credential/egress path.
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

	// This lab has no local workload; the sandbox is an external metric
	// producer. FakeIntake is opt-out here (real E2E by default).
	env.DisableFakeIntake()

	if !params.deployAgent {
		env.DisableAgent()
		return nil
	}

	if params.sapAPIKey == "" {
		return fmt.Errorf(
			"SAP Business Accelerator Hub APIKey is required: export %s=<hub api key> "+
				"before running `dda inv aws.integrations.sap_alm.create` "+
				"(get a free key from https://api.sap.com)",
			apiKeyEnvVar,
		)
	}

	// Readiness gate: verify the host can reach the SAP Cloud ALM sandbox with
	// the supplied APIKey BEFORE the Agent (and its check) start scraping. The
	// APIKey is passed via the command environment, never inlined into the
	// command string or logged. On failure, dump diagnostics to stderr so an
	// opaque connection/401 loop at create becomes a legible root cause.
	//
	// The probe hits the Analytics *data* endpoint (POST /analytics/providers/data)
	// with a minimal time-series query for the EXM_DATAPROVIDER provider, which
	// serves demo data on the public sandbox tenant. We deliberately do NOT probe
	// the GET /analytics/providers catalog: on the shared sandbox demo tenant that
	// endpoint returns HTTP 502 ("No data provider found" for an internal provider),
	// so gating on it would wedge every deploy even though the data endpoint works.
	egressReady, err := host.OS.Runner().Command(
		"sap-alm-sandbox-egress-ready",
		&command.Args{
			Environment: pulumi.StringMap{"SAP_ALM_API_KEY": pulumi.String(params.sapAPIKey)},
			Create: pulumi.String(fmt.Sprintf(`set -eu
url="%s/calm-analytics/v1/analytics/providers/data"
body='{"format":"time_series","timestampFormat":"unix","timeRange":{"semantic":"L1D"},"resolution":"H","queries":[{"name":"probe","provider":"EXM_DATAPROVIDER","columns":{"dimensions":[],"metrics":[]}}]}'
for attempt in $(seq 1 30); do
  code=$(curl -sS -o /tmp/sap_alm_probe.out -w '%%{http_code}' \
    -X POST \
    -H "APIKey: ${SAP_ALM_API_KEY}" \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    --data "${body}" \
    --max-time 20 "${url}" || echo "000")
  if [ "${code}" = "200" ]; then
    echo "SAP Cloud ALM sandbox reachable (HTTP 200 from analytics data endpoint)"
    exit 0
  fi
  echo "attempt ${attempt}: HTTP ${code} from ${url}" >&2
  sleep 5
done
echo "FAILED to reach SAP Cloud ALM sandbox at ${url}" >&2
echo "--- last response body (first 500 bytes) ---" >&2
head -c 500 /tmp/sap_alm_probe.out >&2 || true
echo "" >&2
echo "--- curl version ---" >&2
curl --version >&2 || true
echo "--- DNS/connectivity ---" >&2
getent hosts sandbox.api.sap.com >&2 || true
echo "Hint: HTTP 401/403 means the APIKey is invalid; HTTP 000 means egress to sandbox.api.sap.com is blocked" >&2
exit 1
`, sandboxURL)),
		},
	)
	if err != nil {
		return err
	}

	// Stage the custom check .py and conf.yaml on the host. The files are
	// base64-embedded to avoid shell-quoting hazards and never contain the
	// APIKey (conf.yaml uses the %%env_SAP_ALM_API_KEY%% Autodiscovery
	// template resolved from the container env). This runs after egress is
	// confirmed and before the Agent starts, so the container mounts populated
	// files. On failure it dumps diagnostics before exiting non-zero.
	checkB64 := base64.StdEncoding.EncodeToString([]byte(checkSource))
	confB64 := base64.StdEncoding.EncodeToString([]byte(checkConfig))
	writeAssets, err := host.OS.Runner().Command(
		"sap-alm-write-check-assets",
		&command.Args{
			Sudo: true,
			Create: pulumi.String(fmt.Sprintf(`set -eu
sudo install -d -m 0755 %q %q
if ! printf '%%s' '%s' | base64 -d | sudo tee %q >/dev/null; then
  echo "FAILED to write check source to %s" >&2
  sudo ls -l /opt/sap_alm/checks.d >&2 || true
  exit 1
fi
if ! printf '%%s' '%s' | base64 -d | sudo tee %q >/dev/null; then
  echo "FAILED to write check conf to %s" >&2
  sudo ls -l /opt/sap_alm/conf.d/sap_alm.d >&2 || true
  exit 1
fi
sudo chmod 0644 %q %q
echo "wrote sap_alm check assets"
sudo ls -l %q %q >&2
`,
				"/opt/sap_alm/checks.d", "/opt/sap_alm/conf.d/sap_alm.d",
				checkB64, hostCheckPath, hostCheckPath,
				confB64, hostConfPath, hostConfPath,
				hostCheckPath, hostConfPath,
				hostCheckPath, hostConfPath)),
			Triggers: pulumi.Array{pulumi.String(checkB64), pulumi.String(confB64)},
		},
		utils.PulumiDependsOn(egressReady),
	)
	if err != nil {
		return err
	}

	agentOptions := []dockeragentparams.Option{
		dockeragentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
		// Inject the APIKey into the Agent container so the custom sap_alm
		// check's conf.yaml can reference it (e.g. %%env_SAP_ALM_API_KEY%%).
		dockeragentparams.WithAgentServiceEnvVariable("SAP_ALM_API_KEY", pulumi.String(params.sapAPIKey)),
		// Mount the host-staged custom check + conf into the Agent container.
		dockeragentparams.WithExtraVolumes(
			hostCheckPath+":"+containerCheckPath+":ro",
			hostConfPath+":"+containerConfPath+":ro",
		),
		// Do not start scraping until sandbox egress + auth are confirmed and
		// the check assets are written to the host.
		dockeragentparams.WithPulumiDependsOn(
			utils.PulumiDependsOn(egressReady),
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
