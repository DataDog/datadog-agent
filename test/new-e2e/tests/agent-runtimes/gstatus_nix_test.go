// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentruntimes contains e2e tests for agent runtime components.
package agentruntimes

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// gstatusPath is the path to the agent's bundled gstatus script.
const gstatusPath = "/opt/datadog-agent/embedded/sbin/gstatus"

// gstatusJSON represents the top-level JSON structure that gstatus -o json produces.
type gstatusJSON struct {
	LastUpdated string `json:"last_updated"`
	Data        struct {
		ClusterStatus  string `json:"cluster_status"`
		GlfsVersion    string `json:"glfs_version"`
		NodeCount      int    `json:"node_count"`
		NodesActive    int    `json:"nodes_active"`
		VolumeCount    int    `json:"volume_count"`
		VolumesStarted int    `json:"volumes_started"`
		VolumeSummary  []struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Status    string `json:"status"`
			NumBricks int    `json:"num_bricks"`
			Health    string `json:"health"`
		} `json:"volume_summary"`
	} `json:"data"`
}

type gstatusSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestGstatusAgainstRealGluster(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &gstatusSuite{},
		e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
			awshost.WithRunOptions(
				ec2.WithEC2InstanceOptions(ec2.WithOS(os.Ubuntu2204)),
			),
		)),
	)
}

// SetupSuite installs GlusterFS on the host, starts glusterd, and creates a
// replicated volume with local bricks so that gstatus has real data to report.
func (s *gstatusSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	host := s.Env().RemoteHost

	// Install GlusterFS server. Ubuntu 22.04 ships glusterfs-server in its
	// default repos, so no extra repo config is needed.
	host.MustExecute("sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq")
	host.MustExecute("sudo DEBIAN_FRONTEND=noninteractive apt-get install -y glusterfs-server")

	// Start glusterd.
	host.MustExecute("sudo systemctl enable --now glusterd")

	// Wait for glusterd to be ready.
	require.Eventually(s.T(), func() bool {
		_, err := host.Execute("sudo gluster peer status")
		return err == nil
	}, 30*time.Second, 2*time.Second, "glusterd did not become ready")

	// Create brick directories on the local filesystem.
	host.MustExecute("sudo mkdir -p /data/brick1/gv0 /data/brick2/gv0")

	// Create and start a replicated volume across two local bricks.
	// The volume name is gv0, matching the integrations-core glusterfs test.
	host.MustExecute("sudo gluster volume create gv0 replica 2 " +
		"localhost:/data/brick1/gv0 localhost:/data/brick2/gv0 force")
	host.MustExecute("sudo gluster volume start gv0")
}

// TestGstatusProducesValidJSON runs the agent's bundled gstatus script against
// the real GlusterFS cluster and verifies it produces valid JSON with the
// expected structure.
func (s *gstatusSuite) TestGstatusProducesValidJSON() {
	host := s.Env().RemoteHost

	// Verify the gstatus script exists in the agent package.
	host.MustExecute("test -f " + gstatusPath)

	// Run gstatus -a -o json -u g (the exact invocation the Datadog
	// glusterfs integration uses).
	output, err := host.Execute("sudo " + gstatusPath + " -a -o json -u g")
	require.NoError(s.T(), err, "gstatus failed to run")
	require.NotEmpty(s.T(), output, "gstatus produced no output")

	// gstatus may print a non-JSON warning line before the JSON body
	// (e.g. "Note: Unable to get self-heal status..."). Strip leading
	// non-JSON lines, matching the integrations-core test's approach.
	jsonOutput := output
	if !strings.HasPrefix(strings.TrimSpace(jsonOutput), "{") {
		idx := strings.Index(jsonOutput, "{")
		require.NotEqual(s.T(), -1, idx, "no JSON object found in gstatus output")
		jsonOutput = jsonOutput[idx:]
	}

	// Parse the JSON and verify the structure.
	var result gstatusJSON
	err = json.Unmarshal([]byte(jsonOutput), &result)
	require.NoError(s.T(), err, "gstatus output is not valid JSON")

	// Verify cluster-level fields.
	assert.Equal(s.T(), "Healthy", result.Data.ClusterStatus,
		"cluster should be healthy with a single-node setup")
	assert.NotEmpty(s.T(), result.Data.GlfsVersion,
		"gluster version should be populated")
	assert.Equal(s.T(), 1, result.Data.NodeCount,
		"single-node cluster should have 1 node")
	assert.Equal(s.T(), 1, result.Data.NodesActive,
		"single-node cluster should have 1 active node")
	assert.Equal(s.T(), 1, result.Data.VolumeCount,
		"one volume should exist")
	assert.Equal(s.T(), 1, result.Data.VolumesStarted,
		"one volume should be started")

	// Verify volume summary.
	require.Len(s.T(), result.Data.VolumeSummary, 1, "one volume in summary")
	vol := result.Data.VolumeSummary[0]
	assert.Equal(s.T(), "gv0", vol.Name)
	assert.Equal(s.T(), "Started", vol.Status)
	assert.Equal(s.T(), 2, vol.NumBricks, "replicated volume with 2 bricks")
}

// TestGstatusScriptUsesEmbeddedPython verifies the gstatus script's shebang
// points at the agent's embedded Python, not a system Python. This is a
// packaging-level check that doesn't require glusterd.
func (s *gstatusSuite) TestGstatusScriptUsesEmbeddedPython() {
	host := s.Env().RemoteHost

	output, err := host.Execute("head -1 " + gstatusPath)
	require.NoError(s.T(), err)
	assert.Contains(s.T(), output, "/opt/datadog-agent/embedded/bin/python",
		"gstatus shebang should point at the agent's embedded Python")
}

// TestGstatusVendoredPackagesInstalled verifies the vendored glustercli and
// glusterlib packages are installed in the agent's embedded Python
// site-packages.
func (s *gstatusSuite) TestGstatusVendoredPackagesInstalled() {
	host := s.Env().RemoteHost

	// Check that the vendored packages are importable from the agent's Python.
	_, err := host.Execute("/opt/datadog-agent/embedded/bin/python3 -c " +
		"\"from glusterlib.cluster import Cluster; " +
		"from glustercli.cli import volume; print('OK')\"")
	assert.NoError(s.T(), err,
		"vendored glustercli/glusterlib should be importable from the agent's Python")
}
