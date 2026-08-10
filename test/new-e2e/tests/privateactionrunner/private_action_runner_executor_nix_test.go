// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const (
	// Binary and default unix socket for the on-demand executor (split deployment).
	privateActionRunnerBinary     = "/opt/datadog-agent/embedded/bin/privateactionrunner"
	privateActionRunnerConfigPath = "/etc/datadog-agent/datadog.yaml"
	executorSocketPath            = "/opt/datadog-agent/run/par-executor.sock"

	executorListeningLogLine = "Private action runner executor listening on"
	executorReadyLogLine     = "Private action runner executor ready to accept actions"

	// pkg/remoteconfig/state.ProductActionPlatformRunnerKeys
	runnerKeysRCProduct = "AP_RUNNER_KEYS"
)

// mirrors pkg/privateactionrunner/types.RawKey's JSON shape
type rawRCKey struct {
	KeyType string `json:"keyType"`
	Key     []byte `json:"key"`
}

type linuxPrivateActionRunnerExecutorSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestLinuxPrivateActionRunnerExecutorSuite(t *testing.T) {
	t.Parallel()

	config := GenerateTestPrivateActionRunnerConfig(t)

	e2e.Run(t, &linuxPrivateActionRunnerExecutorSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(agentparams.WithAgentConfig(config)),
			),
		),
	))
}

func (s *linuxPrivateActionRunnerExecutorSuite) pushFakeRunnerKeysConfig() {
	t := s.T()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "failed to generate fake runner key")

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err, "failed to marshal fake runner public key")

	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	payload, err := json.Marshal(rawRCKey{KeyType: "ED25519", Key: pubPEM})
	require.NoError(t, err, "failed to marshal fake runner key config payload")

	err = s.Env().FakeIntake.Client().RCAddConfig("", runnerKeysRCProduct, "fake-runner-key", "fake-runner-key", payload)
	require.NoError(t, err, "failed to push fake runner key config to fakeintake")
}

// TestExecutorStartsAndListens launches the on-demand executor subcommand and
// asserts it comes up: the process runs, the gRPC unix socket is created, and
// the log reports the server listening and ready.
func (s *linuxPrivateActionRunnerExecutorSuite) TestExecutorStartsAndListens() {
	host := s.Env().RemoteHost

	s.pushFakeRunnerKeysConfig()

	// run-executor is a foreground subcommand, not the packaged systemd service.
	// Launch it detached as dd-agent so it can bind its socket under
	// /opt/datadog-agent/run and read the agent IPC cert from /etc/datadog-agent.
	// The pid is captured directly (rather than found via pgrep) because pgrep -f
	// matches on the full command line, so it would also match the very shell
	// invocation used to search for it.
	launch := fmt.Sprintf(
		`sudo -u dd-agent bash -c 'nohup %s run-executor --cfgpath=%s </dev/null >/dev/null 2>&1 & echo $!'`,
		privateActionRunnerBinary, privateActionRunnerConfigPath,
	)
	pid := strings.TrimSpace(host.MustExecute(launch))

	// Allow retrying this test on the same host: kill the detached executor and
	// reset the artifacts it leaves behind so a retry doesn't observe stale state.
	s.T().Cleanup(func() {
		_, _ = host.Execute("sudo kill " + pid)
		_, _ = host.Execute("sudo rm -f " + executorSocketPath)
		_, _ = host.Execute("sudo truncate -s 0 " + privateActionRunnerLogFile)
	})

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, "sudo kill -0 "+pid)
	}, 2*time.Minute, 5*time.Second, "executor process should be running")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, "sudo test -S "+executorSocketPath)
	}, 2*time.Minute, 5*time.Second, "executor gRPC socket should exist")

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", executorListeningLogLine, privateActionRunnerLogFile))
	}, 2*time.Minute, 5*time.Second, "executor log should report listening")

	// Readiness depends on the KeysManager receiving its first AP_RUNNER_KEYS
	// remote-config update. The backend director's first fetch of a brand-new
	// product can take well over 2 minutes regardless of the client's poll
	// interval (observed ~2m10s in CI), so this needs a longer budget than the
	// other checks in this test.
	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		host.MustExecuteOn(c, fmt.Sprintf("sudo grep -F %q %s", executorReadyLogLine, privateActionRunnerLogFile))
	}, 5*time.Minute, 5*time.Second, "executor log should report ready")
}
