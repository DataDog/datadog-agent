// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package sbom

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/sbom"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/sbomtargets"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dockerHostSuite runs the shared SBOM assertions against a standalone Docker
// (moby / overlay2) host - no Kubernetes. It reuses sbomTargetsSuite so the
// container + host SBOMs are verified to be identical to the kubeadm runs.
type dockerHostSuite struct {
	sbomTargetsSuite[environments.DockerHost]
}

// TestSBOMDockerHostSuite provisions a RHEL 10 EC2 VM running Docker, deploys the
// Agent as a Docker container configured to scan the daemon's images (overlayfs
// direct scan) and the host, runs the SBOM target images as Docker containers,
// and asserts the same host + container SBOMs as the kubeadm suites. The Agent
// auto-selects the docker collector from the mounted /var/run/docker.sock.
func TestSBOMDockerHostSuite(t *testing.T) {
	prov := awsdocker.Provisioner(awsdocker.WithRunOptions(dockerHostRunOptions()...))
	e2e.Run(t, &dockerHostSuite{}, e2e.WithProvisioner(prov))
}

// dockerHostRunOptions returns the run options of the suite's environment. A
// test that changes the Agent configuration appends options to them, so that
// UpdateEnv keeps the VM, the fakeintake and the SBOM targets.
func dockerHostRunOptions() []scendocker.Option {
	return []scendocker.Option{
		scendocker.WithEC2VMOptions(
			scenec2.WithOS(e2eos.RedHat10),
			scenec2.WithInstanceType("t3.2xlarge"),
		),
		scendocker.WithFakeIntakeOptions(fakeintake.WithMemory(2048), fakeintake.WithRetentionPeriod(sbomHostRetentionPeriod)),
		scendocker.WithAgentOptions(
			// Mount the host root so the Agent can produce a host SBOM and reach
			// the daemon's overlay2 diff dirs (SanitizeHostPath rewrites
			// /var/lib/docker/... to /host/var/lib/docker/...). The daemon socket
			// is already mounted by the docker agent component.
			dockeragentparams.WithExtraVolumes("/:/host:ro"),
			// HOST_ROOT points the Agent at the mounted host root, so the host SBOM
			// scans the host (not the Agent container) and Trivy's default proc/sys/dev
			// skips apply. Without it the scan targets "/" and walks the live
			// /host/proc, failing on the binfmt_misc symlink loop.
			dockeragentparams.WithAgentServiceEnvVariable("HOST_ROOT", pulumi.String("/host")),
			// Enable host + container SBOM with overlayfs direct scan and the os +
			// languages analyzers, matching the kubeadm Helm values so the SBOMs
			// are identical.
			dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_HOST_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_HOST_ANALYZERS", pulumi.String("os languages")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_CONTAINER_IMAGE_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_CONTAINER_IMAGE_ENABLED", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_CONTAINER_IMAGE_OVERLAYFS_DIRECT_SCAN", pulumi.String("true")),
			dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_CONTAINER_IMAGE_ANALYZERS", pulumi.String("os languages")),
			// Run the SBOM target images as long-lived Docker containers so their
			// images are resident and InUse for the Agent to scan.
			dockeragentparams.WithExtraComposeInlineManifest(sbomtargets.DockerComposeManifest()),
		),
	}
}

func (s *dockerHostSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.Fakeintake = s.Env().FakeIntake.Client()
}

// Test00UpAndRunning waits (long timeout, hence the 00 prefix so it runs first)
// for every SBOM target container to be running, so the images are resident in
// the Docker daemon before the SBOM assertions run.
func (s *dockerHostSuite) Test00UpAndRunning() {
	s.EventuallyWithTf(func(c *assert.CollectT) {
		out, err := s.Env().RemoteHost.Execute("docker ps --format '{{.Names}}'")
		require.NoErrorf(c, err, "failed to list running containers")
		for _, t := range sbomtargets.Targets {
			assert.Containsf(c, out, t.Name, "SBOM target container %q is not running yet", t.Name)
		}
	}, 10*time.Minute, 15*time.Second, "SBOM target containers not running")
}

// fallbackPackageDir holds the npm package TestHostSBOMOSFallback plants on
// the host. A host scan with the languages analyzer lists it, and a scan of the
// OS packages alone leaves it out.
const fallbackPackageDir = "/opt/sbom-e2e/node_modules/left-pad"

// TestHostSBOMOSFallback gives the host scan a 1 second budget, far too short
// for the languages analyzer to walk the host. The Agent must fall back to the
// OS packages alone: the host SBOM succeeds with the rpm packages and leaves
// out the npm package planted on the host. The test restores the suite's
// environment when it ends.
func (s *dockerHostSuite) TestHostSBOMOSFallback() {
	host := s.Env().RemoteHost
	_, err := host.Execute(fmt.Sprintf(`sudo mkdir -p %[1]s && echo '{"name": "left-pad", "version": "1.3.0"}' | sudo tee %[1]s/package.json`, fallbackPackageDir))
	s.Require().NoError(err)
	defer func() {
		_, err := host.Execute("sudo rm -rf /opt/sbom-e2e")
		s.NoError(err)
		s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(dockerHostRunOptions()...)))
		s.Fakeintake = s.Env().FakeIntake.Client()
	}()

	// The Agent restarts during UpdateEnv and scans the host once, at
	// startup, so keep only the host SBOMs generated from here on.
	start := time.Now()
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(append(dockerHostRunOptions(),
		scendocker.WithAgentOptions(dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_HOST_SCAN_TIMEOUT", pulumi.String("1"))),
	)...)))
	s.Fakeintake = s.Env().FakeIntake.Client()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		_, err := host.Execute("docker logs datadog-agent 2>&1 | grep -F 'scanning OS packages only'")
		assert.NoErrorf(c, err, "no OS-only fallback in the Agent logs yet")

		ids, err := s.Fakeintake.GetSBOMIDs()
		require.NoErrorf(c, err, "failed to query fakeintake")
		payloads := lo.FlatMap(ids, func(id string, _ int) []*aggregator.SBOMPayload {
			p, err := s.Fakeintake.FilterSBOMs(id)
			assert.NoErrorf(c, err, "failed to query fakeintake")
			return p
		})
		hosts := lo.Filter(payloads, func(p *aggregator.SBOMPayload, _ int) bool {
			return p.GetType() == sbom.SBOMSourceType_HOST_FILE_SYSTEM &&
				p.Status == sbom.SBOMStatus_SUCCESS &&
				p.GetCyclonedx() != nil &&
				p.GetGeneratedAt().AsTime().After(start)
		})
		require.NotEmptyf(c, hosts, "no successful host SBOM yet")

		for _, p := range hosts {
			comps := p.GetCyclonedx().GetComponents()
			for _, name := range []string{"glibc", "bash", "systemd"} {
				comp := findComponent(comps, name)
				if assert.NotNilf(c, comp, "missing host rpm package %q", name) {
					assert.Truef(c, strings.HasPrefix(comp.GetPurl(), "pkg:rpm/redhat/"), "host rpm package %q has purl %q", name, comp.GetPurl())
				}
			}
			assert.Emptyf(c, applicationComponents(comps), "the OS-only host SBOM lists application packages")
		}
	}, 10*time.Minute, 15*time.Second, "no OS-only host SBOM after the application scan ran out of time")
}

// TestZZDumpAgentDiagnostics runs last (the ZZ prefix sorts it after the SBOM
// tests) and dumps Docker + Agent SBOM state that otherwise lives only in the
// flare artifact, not the test trace: the storage driver (the overlay scan
// requires overlay2) and the Agent's SBOM/trivy logs. It is a debugging aid for
// the container SBOM assertions and always passes.
func (s *dockerHostSuite) TestZZDumpAgentDiagnostics() {
	host := s.Env().RemoteHost
	for _, cmd := range []string{
		"docker info --format 'storage-driver={{.Driver}}'",
		"docker exec datadog-agent agent status 2>&1 | grep -iEA2 'sbom|container image' | head -80",
		"docker logs datadog-agent 2>&1 | grep -iE 'sbom|trivy|overlay|scan' | tail -150",
	} {
		out, err := host.Execute(cmd)
		s.T().Logf("DIAG %q err=%v\n%s", cmd, err, out)
	}
}
