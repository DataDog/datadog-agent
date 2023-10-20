// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	_ "embed"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
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

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

type multiFakeIntakeEnv struct {
	VM          *client.VM
	Agent       *client.Agent
	Fakeintake1 *client.Fakeintake
	Fakeintake2 *client.Fakeintake
}

const (
	// intakeName should only contains alphanumerical characters to ease pattern matching /etc/hosts
	intakeName              = "ddintake"
	connectionResetInterval = 120 // seconds
	intakeMaxWaitTime       = 5 * time.Minute
	intakeTick              = 20 * time.Second
	fakeintake1Name         = "1fakeintake"
	fakeintake2Name         = "2fakeintake"
)

//go:embed testfixtures/custom_logs.yaml
var customLogsConfig string

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
	e2e.Run(t, &multiFakeIntakeSuite{}, multiFakeintakeStackDef())
}

// SetupSuite waits for both fakeintakes to be ready before running tests.
func (v *multiFakeIntakeSuite) SetupSuite() {
	v.Env() // update the environment outside EventuallyWithT

	// Wait for the fakeintake to be ready to avoid 503
	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		assert.NoError(c, v.Env().Fakeintake1.Client.GetServerHealth())
		assert.NoError(c, v.Env().Fakeintake2.Client.GetServerHealth())
	}, intakeMaxWaitTime, intakeTick)
}

// BeforeTest flushes both fakeintakes before starting each test.
func (v *multiFakeIntakeSuite) SetupTest() {
	v.NoError(v.Env().Fakeintake1.FlushServerAndResetAggregators())
	v.NoError(v.Env().Fakeintake2.FlushServerAndResetAggregators())
}

// TestDNSFailover tests that the agent correctly picks-up a change in the DNS entry of the intake.
//
// The test uses two fakeintakes to represent two backends, and the /etc/hosts file as a DNS,
// setting-up an entry for the intake, pointing to the first fakeintake, then changing that entry
// to point to the second fakeintake without restarting the agent.
//
// The test checks that metrics, logs, and flares are properly received by the first intake before
// the failover, and by the second one after.
//
// TODO: handle APM traces
func (v *multiFakeIntakeSuite) TestDNSFailover() {
	// setup local version of DNS entry for intake
	fakeintake1IP, err := hostIPFromURL(v.Env().Fakeintake1.URL())
	v.NoError(err)
	setHostEntry(v.T(), v.Env().VM, intakeName, fakeintake1IP)

	// configure agent to use the custom intake, set connection_reset_interval, use logs, and processes
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(getAgentConfig(intakeName, connectionResetInterval)),
		agentparams.WithLogs(),
		agentparams.WithIntegration("custom_logs.d", customLogsConfig),
	}
	v.UpdateEnv(multiFakeintakeStackDef(agentOptions...))

	// check that fakeintake1 does receive metrics
	v.T().Logf("checking that the agent contacts main intake at %s", fakeintake1IP)
	require.EventuallyWithT(
		v.T(),
		assertIntakeIsUsed(v.Env().VM, v.Env().Fakeintake1, v.Env().Agent),
		intakeMaxWaitTime,
		intakeTick,
	)

	// check that fakeintake2 doesn't receive metrics
	assertIntakeNotUsed(v.T(), v.Env().VM, v.Env().Fakeintake2, v.Env().Agent)

	// perform local version of DNS failover
	fakeintake2IP, err := hostIPFromURL(v.Env().Fakeintake2.URL())
	v.NoError(err)
	setHostEntry(v.T(), v.Env().VM, intakeName, fakeintake2IP)

	// check that fakeintake2 receives metrics
	v.T().Logf("checking that the agent contacts fallback intake at %s", fakeintake2IP)
	require.EventuallyWithT(
		v.T(),
		assertIntakeIsUsed(v.Env().VM, v.Env().Fakeintake2, v.Env().Agent),
		connectionResetInterval*time.Second+intakeMaxWaitTime,
		intakeTick,
	)
}

// assertIntakeIsUsed asserts the the given intakes receives metrics, logs, and flares
func assertIntakeIsUsed(vm *client.VM, intake *client.Fakeintake, agent *client.Agent) func(*assert.CollectT) {
	return func(t *assert.CollectT) {
		// check metrics
		metricNames, err := intake.GetMetricNames()
		require.NoError(t, err)
		assert.NotEmpty(t, metricNames)

		// check logs
		vm.Execute("echo 'totoro' >> /tmp/test.log")
		logs, err := intake.FilterLogs("custom_logs")
		require.NoError(t, err)
		assert.NotEmpty(t, logs)

		// check flares
		agent.Flare(client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))
		_, err = intake.GetLatestFlare()
		if err != nil {
			require.ErrorIs(t, err, fi.ErrNoFlareAvailable)
		}
		assert.NoError(t, err)
	}
}

// assertIntakeNotUsed asserts that the given intake doesn't receive metrics, logs, and flares
func assertIntakeNotUsed(t *testing.T, vm *client.VM, intake *client.Fakeintake, agent *client.Agent) {
	// check metrics
	metricNames, err := intake.GetMetricNames()
	require.NoError(t, err)
	require.Empty(t, metricNames)

	// check logs
	vm.Execute("echo 'totoro' >> /tmp/test.log")
	logs, err := intake.FilterLogs("custom_logs")
	require.NoError(t, err)
	require.Empty(t, logs)

	// check flares
	agent.Flare(client.WithArgs([]string{"--email", "e2e@test.com", "--send"}))
	_, err = intake.GetLatestFlare()
	require.ErrorIs(t, err, fi.ErrNoFlareAvailable)
}

// setHostEntry adds an entry in /etc/hosts for the given hostname and hostIP
// if there is already an entry for that hostname, it is replaced
func setHostEntry(t *testing.T, vm *client.VM, hostname string, hostIP string) {
	// we could remove the line and then add the new one,
	// but it's better to avoid not having the line in the file between the two operations

	t.Logf("set host entry for %s: %s", hostname, hostIP)

	hostfile := vm.Execute("sudo cat /etc/hosts")

	// pattern to match the hostname entry
	hostPattern := fmt.Sprintf("^.* %s$", regexp.QuoteMeta(hostname))
	// enable multi-line mode in the Go regex
	goHostPattern := fmt.Sprintf("(?m:%s)", hostPattern)
	matched, err := regexp.MatchString(goHostPattern, hostfile)
	require.NoError(t, err)

	entry := fmt.Sprintf("%s %s", hostIP, hostname)
	if matched {
		t.Logf("replace existing host entry for %s (%s)", hostname, hostIP)
		vm.Execute(fmt.Sprintf("sudo sed -i 's/%s/%s/g' /etc/hosts", hostPattern, entry))
	} else {
		t.Logf("append new host entry for %s (%s)", hostname, hostIP)
		vm.Execute(fmt.Sprintf("echo '%s' | sudo tee -a /etc/hosts", entry))
	}
}

// hostIPFromURL extracts the host from the given URL and returns any IP associated to that host
// or an error
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

func getAgentConfig(intake string, interval int) string {
	return strings.Join([]string{
		getDDUrlConf(intake),
		getConnectionResetConf(interval),
	}, "\n")
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
