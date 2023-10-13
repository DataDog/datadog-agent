// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	awsResources "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
)

type multiFakeIntakeEnv struct {
	VM          *client.VM
	Agent       *client.Agent
	Fakeintake1 *client.Fakeintake
	Fakeintake2 *client.Fakeintake
}

const (
	// should only contains alphanumerical characters to ease pattern matching /etc/hosts
	intakeName              = "ddintake"
	connectionResetInterval = 120 // seconds
	intakeMaxWaitTime       = 5 * time.Minute
	intakeTick              = 10 * time.Second
	fakeintake1Name         = "1fakeintake"
	fakeintake2Name         = "2fakeintake"
)

// for local dev purpose
var testParams = []params.Option{
	params.WithDevMode(),
	params.WithStackName("pgimalac-examples-multifakeintakesuite-000002"),
}

func multiFakeintakeStackDef(agentOptions ...agentparams.Option) *e2e.StackDefinition[multiFakeIntakeEnv] {
	return e2e.EnvFactoryStackDef(func(ctx *pulumi.Context) (*multiFakeIntakeEnv, error) {
		awsEnv, err := awsResources.NewEnvironment(ctx)
		if err != nil {
			return nil, err
		}

		vm, err := ec2vm.NewEC2VMWithEnv(awsEnv)
		if err != nil {
			return nil, err
		}

		fiExporter1, err := aws.NewEcsFakeintakeWithName(awsEnv, fakeintake1Name)
		if err != nil {
			return nil, err
		}

		fiExporter2, err := aws.NewEcsFakeintakeWithName(awsEnv, fakeintake2Name)
		if err != nil {
			return nil, err
		}

		agentInstaller, err := agent.NewInstaller(vm, agentOptions...)
		if err != nil {
			return nil, err
		}

		return &multiFakeIntakeEnv{
			VM:          client.NewVM(vm),
			Agent:       client.NewAgent(agentInstaller),
			Fakeintake1: client.NewFakeintake(fiExporter1),
			Fakeintake2: client.NewFakeintake(fiExporter2),
		}, nil
	})
}

type multiFakeIntakeSuite struct {
	e2e.Suite[multiFakeIntakeEnv]
}

func TestMultiFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &multiFakeIntakeSuite{}, multiFakeintakeStackDef(), testParams...)
}

func (v *multiFakeIntakeSuite) TestCleanup() {
	v.NoError(v.Env().Fakeintake1.FlushServerAndResetAggregators())
	v.NoError(v.Env().Fakeintake2.FlushServerAndResetAggregators())
}

func (v *multiFakeIntakeSuite) TestDNSFailover() {
	// setup local version of DNS entry for intake
	fakeintake1IP, err := hostIPFromURL(v.Env().Fakeintake1.URL())
	v.NoError(err)
	setHostEntry(v.T(), v.Env().VM, intakeName, fakeintake1IP)

	// configure agent to use the custom intake and set connection_reset_interval
	agentConfig := fmt.Sprintf("%s\n%s", getDDUrlConf(intakeName), getConnectionResetConf(connectionResetInterval))
	v.UpdateEnv(multiFakeintakeStackDef(agentparams.WithAgentConfig(agentConfig)))

	// check that fakeintake1 does receive metrics
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().Fakeintake1.GetMetricNames()
		require.NoError(c, err)
		require.NotEmpty(c, metricNames)
	}, intakeMaxWaitTime, intakeTick)

	// check that fakeintake2 doesn't receive metrics
	metricNames, err := v.Env().Fakeintake2.GetMetricNames()
	v.NoError(err)
	v.Empty(metricNames)

	// perform local version of DNS failover
	fakeintake2IP, err := hostIPFromURL(v.Env().Fakeintake2.URL())
	v.NoError(err)
	setHostEntry(v.T(), v.Env().VM, intakeName, fakeintake2IP)

	// check that fakeintake2 receives metrics
	v.EventuallyWithT(func(c *assert.CollectT) {
		metricNames, err := v.Env().Fakeintake2.GetMetricNames()
		require.NoError(c, err)
		require.NotEmpty(c, metricNames)
	}, connectionResetInterval*time.Second+intakeMaxWaitTime, intakeTick)
}

func setHostEntry(t *testing.T, vm *client.VM, hostname string, hostIP string) {
	// we could remove the line and then add the new one,
	// but it's better to avoid not having the line in the file between the two operations

	hostfile := vm.Execute("sudo cat /etc/hosts")
	hostPattern := fmt.Sprintf("^.* %s$", hostname)
	matched, err := regexp.MatchString(hostPattern, hostfile)
	require.NoError(t, err)

	entry := fmt.Sprintf("%s %s", hostIP, hostname)
	if matched {
		vm.Execute(fmt.Sprintf("sudo sed -i 's/%s/%s/g' /etc/hosts", hostPattern, entry))
	} else {
		vm.Execute(fmt.Sprintf("echo '%s' | sudo tee -a /etc/hosts", entry))
	}
}

func hostIPFromURL(fakeintakeURL string) (string, error) {
	parsed, err := url.Parse(fakeintakeURL)
	if err != nil {
		return "", err
	}

	host := parsed.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no ip for host %s", host)
	}

	// return any valid ip
	return ips[0].String(), nil
}

func getDDUrlConf(intake string) string {
	// config from components/datadog/agentparams/params.go::WithFakeintake
	return fmt.Sprintf(`dd_url: http://%s:80
logs_config.logs_dd_url: %s:80
logs_config.logs_no_ssl: true
logs_config.force_use_http: true
process_config.process_dd_url: http://%s:80`, intake, intake, intake)
}

func getConnectionResetConf(interval int) string {
	return fmt.Sprintf(`forwarder_connection_reset_interval: %d
apm_config.connection_reset_interval: %d
logs_config.connection_reset_interval: %d`, interval, interval, interval)
}
