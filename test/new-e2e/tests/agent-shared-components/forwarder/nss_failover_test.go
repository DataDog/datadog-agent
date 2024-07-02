// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forwarder

import (
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type multiFakeIntakeEnv struct {
	Host        *components.RemoteHost
	Agent       *components.RemoteHostAgent
	Fakeintake1 *components.FakeIntake
	Fakeintake2 *components.FakeIntake
}

func (e *multiFakeIntakeEnv) Init(ctx e2e.Context) error {
	if e.Agent != nil {
		agent, err := client.NewHostAgentClient(ctx, e.Host.HostOutput, true)
		if err != nil {
			return err
		}
		e.Agent.Client = agent
	}

	return nil
}

const (
	// intakeName should only contains alphanumerical characters to ease pattern matching /etc/hosts
	intakeName      = "ddintake"
	fakeintake1Name = "1"
	fakeintake2Name = "2"

	logFile                 = "/tmp/test.log"
	logService              = "custom_logs"
	connectionResetInterval = 120 // seconds

	intakeMaxWaitTime    = 5 * time.Minute
	intakeUnusedWaitTime = 1 * time.Minute
	intakeTick           = 20 * time.Second
)

// templateVars is used to template the configs
var templateVars = map[string]string{
	"ConnectionResetInterval": strconv.Itoa(connectionResetInterval),
	"LogFile":                 logFile,
	"LogService":              logService,
}

//go:embed testfixtures/custom_logs.yaml.tmpl
var customLogsConfigTmplFile string

//go:embed testfixtures/config.yaml.tmpl
var configTmplFile string

func multiFakeIntakeAWS(agentOptions ...agentparams.Option) e2e.Provisioner {
	runFunc := func(ctx *pulumi.Context, env *multiFakeIntakeEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, "nssfailover")
		if err != nil {
			return err
		}
		host.Export(ctx, &env.Host.HostOutput)

		agent, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}
		agent.Export(ctx, &env.Agent.HostAgentOutput)

		fakeIntake1, err := fakeintake.NewECSFargateInstance(awsEnv, fakeintake1Name)
		if err != nil {
			return err
		}
		fakeIntake1.Export(ctx, &env.Fakeintake1.FakeintakeOutput)

		fakeIntake2, err := fakeintake.NewECSFargateInstance(awsEnv, fakeintake2Name)
		if err != nil {
			return err
		}
		fakeIntake2.Export(ctx, &env.Fakeintake2.FakeintakeOutput)

		return nil
	}

	return e2e.NewTypedPulumiProvisioner("aws-nssfailover", runFunc, nil)
}

type multiFakeIntakeSuite struct {
	e2e.BaseSuite[multiFakeIntakeEnv]
}

func TestMultiFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &multiFakeIntakeSuite{}, e2e.WithProvisioner(multiFakeIntakeAWS()))
}

// TestNSSFailover tests that the agent correctly picks-up an NSS change of the intake.
//
// The test uses two fakeintakes to represent two backends, and the /etc/hosts file for the NSS source,
// setting-up an entry for the intake, pointing to the first fakeintake, then changing that entry
// to point to the second fakeintake without restarting the agent.
//
// The test checks that metrics, logs, and flares are properly received by the first intake before
// the failover, and by the second one after.
//
// Note: although the man page of `nsswitch.conf` states that each process using it should only read
// it once (ie. no reload), glibc and Go reload it periodically
// cf. https://go-review.googlesource.com/c/go/+/448075
//
// TODO: handle APM traces
func (v *multiFakeIntakeSuite) TestNSSFailover() {
	agentConfig, err := readTmplConfig(configTmplFile)
	v.NoError(err)

	customLogsConfig, err := readTmplConfig(customLogsConfigTmplFile)
	v.NoError(err)

	// ensure host uses files for NSS
	enforceNSSwitchFiles(v.T(), v.Env().Host)

	// setup NSS entry for intake
	fakeintake1IP, err := hostIPFromURL(v.Env().Fakeintake1.URL)
	v.NoError(err)
	setHostEntry(v.T(), v.Env().Host, intakeName, fakeintake1IP)

	// configure agent to use the custom intake, set connection_reset_interval, use logs, and processes
	agentOptions := []agentparams.Option{
		agentparams.WithAgentConfig(agentConfig),
		agentparams.WithLogs(),
		agentparams.WithIntakeHostname(intakeName),
		agentparams.WithIntegration("custom_logs.d", customLogsConfig),
	}
	v.UpdateEnv(multiFakeIntakeAWS(agentOptions...))

	// check that fakeintake1 is used as intake and not fakeintake2
	v.requireIntakeIsUsed(v.Env().Fakeintake1.Client(), intakeMaxWaitTime, intakeTick)
	v.requireIntakeNotUsed(v.Env().Fakeintake2.Client(), intakeMaxWaitTime, intakeTick)

	// perform NSS change
	fakeintake2IP, err := hostIPFromURL(v.Env().Fakeintake2.URL)
	v.NoError(err)
	setHostEntry(v.T(), v.Env().Host, intakeName, fakeintake2IP)

	// check that fakeintake2 is used as intake and not fakeintake1
	intakeMaxWaitTime := connectionResetInterval*time.Second + intakeMaxWaitTime
	v.requireIntakeIsUsed(v.Env().Fakeintake2.Client(), intakeMaxWaitTime, intakeTick)
	v.requireIntakeNotUsed(v.Env().Fakeintake1.Client(), intakeMaxWaitTime, intakeTick)
}

// requireIntakeIsUsed checks that the given intakes receives metrics, logs, and flares
func (v *multiFakeIntakeSuite) requireIntakeIsUsed(intake *fi.Client, intakeMaxWaitTime, intakeTick time.Duration) {
	checkFn := func(t *assert.CollectT) {
		// check metrics
		metricNames, err := intake.GetMetricNames()
		require.NoError(t, err)
		assert.NotEmpty(t, metricNames)

		// check logs
		v.Env().Host.MustExecute(fmt.Sprintf("echo 'totoro' >> %s", logFile))
		logs, err := intake.FilterLogs(logService)
		require.NoError(t, err)
		assert.NotEmpty(t, logs)

		// check flares
		v.Env().Agent.Client.Flare(agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"}))
		_, err = intake.GetLatestFlare()
		if err != nil {
			require.ErrorIs(t, err, fi.ErrNoFlareAvailable)
		}
		assert.NoError(t, err)
	}

	v.T().Logf("checking that the agent contacts intake at %s", intake.URL())
	require.EventuallyWithT(v.T(), checkFn, intakeMaxWaitTime, intakeTick)
}

// requireIntakeNotUsed checks that the given intake doesn't receive metrics, logs, and flares
func (v *multiFakeIntakeSuite) requireIntakeNotUsed(intake *fi.Client, intakeMaxWaitTime, intakeTick time.Duration) {
	checkFn := func(t *assert.CollectT) {
		// flush intake
		intake.FlushServerAndResetAggregators()

		// write a log
		v.Env().Host.MustExecute(fmt.Sprintf("echo 'totoro' >> %s", logFile))

		// send a flare
		v.Env().Agent.Client.Flare(agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

		// give time to the agent to send things
		time.Sleep(intakeUnusedWaitTime)

		stats, err := intake.RouteStats()
		require.NoError(t, err)

		assert.Empty(t, stats)
	}

	v.T().Logf("checking that the agent doesn't contact intake at %s", intake.URL())
	require.EventuallyWithT(v.T(), checkFn, intakeMaxWaitTime, intakeTick)
}

// setHostEntry adds an entry in /etc/hosts for the given hostname and hostIP
// if there is already an entry for that hostname, it is replaced
func setHostEntry(t *testing.T, host *components.RemoteHost, hostname string, hostIP string) {
	// we could remove the line and then add the new one,
	// but it's better to avoid not having the line in the file between the two operations

	t.Logf("set host entry for %s: %s", hostname, hostIP)

	hostfile := host.MustExecute("sudo cat /etc/hosts")

	// pattern to match the hostname entry
	hostPattern := fmt.Sprintf("^.* %s$", regexp.QuoteMeta(hostname))
	// enable multi-line mode in the Go regex
	goHostPattern := fmt.Sprintf("(?m:%s)", hostPattern)
	matched, err := regexp.MatchString(goHostPattern, hostfile)
	require.NoError(t, err)

	entry := fmt.Sprintf("%s %s", hostIP, hostname)
	if matched {
		t.Logf("replace existing host entry for %s (%s)", hostname, hostIP)
		host.MustExecute(fmt.Sprintf("sudo sed -i 's/%s/%s/g' /etc/hosts", hostPattern, entry))
	} else {
		t.Logf("append new host entry for %s (%s)", hostname, hostIP)
		host.MustExecute(fmt.Sprintf("echo '%s' | sudo tee -a /etc/hosts", entry))
	}
}

// enforceNSSwitchFiles ensures /etc/nsswitch.conf uses `files` first for the `hosts` entry
// so that an NSS query uses /etc/hosts before DNS
func enforceNSSwitchFiles(t *testing.T, host *components.RemoteHost) {
	// for the specifics of the nsswitch.conf file format, see its man page
	//
	// in short, the hosts line starts with "hosts:", then a whitespace separated list of "services"
	// each service can be followed by actions in the format [STATUS=ACTION] or [!STATUS=ACTION]
	// we want to have the "files" service first without any action after

	t.Logf("enforce using files first in NSS")

	nsswitchfile := host.MustExecute("sudo cat /etc/nsswitch.conf")

	// enable multi-line mode in the Go regex
	regex, err := regexp.Compile(`(?m:^hosts:\s+(.*)$)`)
	require.NoError(t, err)

	if regex.MatchString(nsswitchfile) {
		matches := regex.FindStringSubmatch(nsswitchfile)
		require.NotNil(t, matches)

		services := strings.Fields(matches[1])
		if len(services) == 0 || services[0] != "files" || (len(services) >= 2 && services[1][0] == '[') {
			t.Logf("replace existing hosts entry in /etc/nsswitch.conf")
			// add `files` before previous services
			host.MustExecute(`sudo sed -E -i 's/^hosts:(\s+)(.*)$/hosts:\1files \2/g' /etc/nsswitch.conf`)
		}
	} else {
		t.Logf("add hosts entry in /etc/nsswitch.conf")
		host.MustExecute("echo 'hosts: files' | sudo tee -a /etc/nsswitch.conf")
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

func readTmplConfig(tmplContent string) (string, error) {
	var buffer bytes.Buffer

	tmpl, err := template.New("").Parse(tmplContent)
	if err != nil {
		return "", err
	}

	err = tmpl.Execute(&buffer, templateVars)
	if err != nil {
		return "", err
	}

	return buffer.String(), nil
}
