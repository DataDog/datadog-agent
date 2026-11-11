// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pretest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

// verboseOutput prints 300,000 bytes on stderr, more than sshd takes in one
// packet (256 KiB) and exec in one environment string (128 KiB).
const verboseOutput = `head -c 300000 /dev/zero | tr '\0' x | fold -w 99 >&2`

type commandOutputSuite struct {
	e2e.BaseSuite[environments.Host]
	localRunFile string
}

func TestCommandOutputSuite(t *testing.T) {
	s := &commandOutputSuite{localRunFile: filepath.Join(t.TempDir(), "run")}
	e2e.Run(t, s, e2e.WithProvisioner(s.verboseCommands("first")), e2e.WithSkipCoverage())
}

// TestUpdateVerboseCommands updates a remote and a local command whose last
// run printed verboseOutput.
func (s *commandOutputSuite) TestUpdateVerboseCommands() {
	s.UpdateEnv(s.verboseCommands("second"))

	s.Equal("second\n", s.Env().RemoteHost.MustExecute("cat /tmp/verbose-command"))
	local, err := os.ReadFile(s.localRunFile)
	s.Require().NoError(err)
	s.Equal("second\n", string(local))
}

// verboseCommands provisions a VM with a remote and a local command that print
// verboseOutput, then write run to a file.
func (s *commandOutputSuite) verboseCommands(run string) provisioners.Provisioner {
	return awshost.ProvisionerNoAgentNoFakeIntake(awshost.WithRunOptions(ec2.WithPreAgentInstallHook(
		func(e *aws.Environment, host *remote.Host) (pulumi.Resource, error) {
			localRunner := command.NewLocalRunner(e, command.LocalRunnerArgs{OSCommand: command.NewLocalOSCommand()})
			if _, err := localRunner.Command("verbose", &command.Args{
				Create: pulumi.String(verboseOutput + "; echo " + run + " >" + s.localRunFile),
			}); err != nil {
				return nil, err
			}
			return host.OS.Runner().Command("verbose", &command.Args{
				Create: pulumi.String(verboseOutput + "; echo " + run + " >/tmp/verbose-command"),
			})
		})))
}
