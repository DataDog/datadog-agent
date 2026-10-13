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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/sbomtargets"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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

	// A run of TestZZZHostSBOMOSFallback cut short leaves its files on the
	// host, which slow down every host scan, the one the Agent ran at startup
	// included. Remove them, restart the Agent, and drop its SBOMs.
	out := s.Env().RemoteHost.MustExecute(fmt.Sprintf(`sudo sh -c 'if [ -d %[1]s ]; then rm -rf %[1]s && echo removed; fi'`, fallbackRoot))
	if strings.TrimSpace(out) == "removed" {
		s.Env().RemoteHost.MustExecute("docker restart datadog-agent")
		s.Require().NoError(s.Fakeintake.FlushServerAndResetAggregators())
	}
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

// fallbackPackageDir holds the npm package TestZZZHostSBOMOSFallback plants on
// the host. A host scan with the languages analyzer lists it, and a scan of the
// OS packages alone leaves it out.
const fallbackPackageDir = fallbackRoot + "/node_modules/left-pad"

// fallbackSlowDir holds a package.json padded to 1 MiB and 8192 hard links to
// it, which TestZZZHostSBOMOSFallback plants on the host. The languages analyzer
// parses the file at each of its paths, which takes far longer than a second.
const fallbackSlowDir = fallbackRoot + "/slow"

// fallbackRoot holds the files TestZZZHostSBOMOSFallback plants on the host.
const fallbackRoot = "/opt/sbom-e2e"

// TestZZZHostSBOMOSFallback gives the host scan a 1 second budget, far too
// short for the languages analyzer to parse the files planted on the host. The
// Agent must fall back to the OS packages alone: the host SBOM succeeds with
// the rpm packages, leaves out the npm package planted on the host, and carries
// the fallback property. The ZZZ prefix runs it last, since BaseSuite recreates
// the Agent to restore the suite's environment before the next test.
func (s *dockerHostSuite) TestZZZHostSBOMOSFallback() {
	host := s.Env().RemoteHost
	host.MustExecute(fmt.Sprintf(`sudo mkdir -p %[1]s && echo '{"name": "left-pad", "version": "1.3.0"}' | sudo tee %[1]s/package.json`, fallbackPackageDir))
	defer host.MustExecute("sudo rm -rf " + fallbackRoot)
	host.MustExecute(fmt.Sprintf(`sudo sh -c 'set -e
		mkdir -p %[1]s/0
		cd %[1]s
		printf "{%%1048576s}" "" >0/package.json
		seq 8192 | xargs -P 8 -I@ cp -al 0 @'`, fallbackSlowDir))

	// The Agent restarts during UpdateEnv and scans the host once, at
	// startup, so keep only the host SBOMs collected from here on.
	start := time.Now()
	s.UpdateEnv(awsdocker.Provisioner(awsdocker.WithRunOptions(append(dockerHostRunOptions(),
		scendocker.WithAgentOptions(dockeragentparams.WithAgentServiceEnvVariable("DD_SBOM_HOST_SCAN_TIMEOUT", pulumi.String("1"))),
	)...)))
	s.Fakeintake = s.Env().FakeIntake.Client()
	// A run of the suite on the same stack must find no fallback SBOM in
	// TestHostSBOM.
	defer func() {
		s.NoError(s.Fakeintake.FlushServerAndResetAggregators())
	}()

	s.EventuallyWithTf(func(c *assert.CollectT) {
		bom := newestHostBOM(c, s.Fakeintake, start)
		assert.Equalf(c, []string{"true"}, propertyValues(bom.GetMetadata().GetProperties(), propOSFallback), "the host SBOM lacks the fallback property")
		comps := bom.GetComponents()
		for _, name := range []string{"glibc", "bash", "systemd"} {
			comp := findComponent(comps, name)
			if assert.NotNilf(c, comp, "missing host rpm package %q", name) {
				assert.Truef(c, strings.HasPrefix(comp.GetPurl(), "pkg:rpm/redhat/"), "host rpm package %q has purl %q", name, comp.GetPurl())
			}
		}
		assert.Emptyf(c, applicationComponents(comps), "the OS-only host SBOM lists application packages")
	}, 10*time.Minute, 15*time.Second, "no OS-only host SBOM after the application scan ran out of time")
}

// TestZZDumpAgentDiagnostics runs after the SBOM tests on the suite's
// environment (the ZZ prefix sorts it after them, and before
// TestZZZHostSBOMOSFallback, which changes it) and dumps Docker + Agent SBOM
// state that otherwise lives only in the flare artifact, not the test trace:
// the storage driver (the overlay scan requires overlay2) and the Agent's
// SBOM/trivy logs. It is a debugging aid for the container SBOM assertions
// and always passes.
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
