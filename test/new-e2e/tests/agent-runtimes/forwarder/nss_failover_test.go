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

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

type multiFakeIntakeEnv struct {
	Host        *components.RemoteHost
	Agent       *components.RemoteHostAgent
	Fakeintake1 *components.FakeIntake
	Fakeintake2 *components.FakeIntake
	Docker      *components.RemoteHostDocker
}

func (e *multiFakeIntakeEnv) Init(ctx common.Context) error {
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
	connectionResetInterval = 20 // seconds

	intakeMaxWaitTime    = 2 * time.Minute
	intakeUnusedWaitTime = 20 * time.Second
	intakeTick           = 5 * time.Second
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

func pullTraceGeneratorImage(h *components.RemoteHost) {
	h.MustExecute("docker pull ghcr.io/datadog/apps-tracegen:" + apps.Version)
}

func runUDSTraceGenerator(h *components.RemoteHost, service string, addSpanTags string) func() {
	rm := "docker rm -f " + service
	h.MustExecute(rm) // kill any existing leftover container

	run := "docker run -d --rm --name " + service +
		" -v /var/run/datadog/:/var/run/datadog/ " +
		" -e DD_TRACE_AGENT_URL=unix:///var/run/datadog/apm.socket " +
		" -e DD_SERVICE=" + service +
		" -e DD_GIT_COMMIT_SHA=abcd1234 " +
		" -e TRACEGEN_ADDSPANTAGS=" + addSpanTags +
		" ghcr.io/datadog/apps-tracegen:" + apps.Version
	h.MustExecute(run)

	return func() { h.MustExecute(rm) }
}

func multiFakeIntakeAWS(agentOptions ...agentparams.Option) provisioners.Provisioner {
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

		// Create a docker manager
		dockerManager, err := docker.NewManager(&awsEnv, host)
		if err != nil {
			return err
		}
		// export the docker manager configuration to the environment, this will automatically initialize the docker client
		err = dockerManager.Export(ctx, &env.Docker.ManagerOutput)
		if err != nil {
			return err
		}

		return nil
	}

	return provisioners.NewTypedPulumiProvisioner("aws-nssfailover", runFunc, nil)
}

type multiFakeIntakeSuite struct {
	e2e.BaseSuite[multiFakeIntakeEnv]
}

func TestMultiFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &multiFakeIntakeSuite{}, e2e.WithProvisioner(multiFakeIntakeAWS()))
}

// BeforeTest ensures that both fakeintakes are not in use before the test starts
//
// This is necessary due to fakeintake IP reuse, sometimes the fakeintake is destroyed before the Agent / host is,
// and the Agent keeps sending payloads to the fakeintake, which can cause errors if the IP is reused too quickly.
// See https://datadoghq.atlassian.net/browse/ACIX-1005.
func (v *multiFakeIntakeSuite) BeforeTest(suiteName, testName string) {
	v.BaseSuite.BeforeTest(suiteName, testName)

	maxWaitTime := 10 * time.Minute

	checkNotUsed := func(intake *fi.Client) {
		require.EventuallyWithT(v.T(), func(t *assert.CollectT) {
			intake.FlushServerAndResetAggregators()

			// give time to the agent to flush to the intake
			time.Sleep(intakeUnusedWaitTime)

			stats, err := intake.RouteStats()
			require.NoError(t, err)
			assert.Empty(t, stats)

		}, maxWaitTime, intakeTick)
	}

	checkNotUsed(v.Env().Fakeintake1.Client())
	checkNotUsed(v.Env().Fakeintake2.Client())
}

// TestNSSFailover tests that the agent correctly picks-up an NSS change of the intake.
//
// The test uses two fakeintakes to represent two backends, and the /etc/hosts file for the NSS source,
// setting-up an entry for the intake, pointing to the first fakeintake, then changing that entry
// to point to the second fakeintake without restarting the agent.
//
// The test checks that metrics, logs, traces, and flares are properly received by the first intake before
// the failover, and by the second one after.
//
// Note: although the man page of `nsswitch.conf` states that each process using it should only read
// it once (ie. no reload), glibc and Go reload it periodically
// cf. https://go-review.googlesource.com/c/go/+/448075
func (v *multiFakeIntakeSuite) TestNSSFailover() {
	// Ensure that both fakeintakes are using the same scheme
	v.Assert().Equal(v.Env().Fakeintake1.Scheme, v.Env().Fakeintake2.Scheme)

	agentConfig, err := readTmplConfig(configTmplFile)
	v.NoError(err)

	customLogsConfig, err := readTmplConfig(customLogsConfigTmplFile)
	v.NoError(err)

	// pull tracegen docker image
	pullTraceGeneratorImage(v.Env().Host)

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
		agentparams.WithIntakeHostname(v.Env().Fakeintake1.Scheme, intakeName),
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

// requireIntakeIsUsed checks that the given intakes receives metrics, logs, traces, and flares
func (v *multiFakeIntakeSuite) requireIntakeIsUsed(intake *fi.Client, intakeMaxWaitTime, intakeTick time.Duration) {
	checkFn := func(t *assert.CollectT) {
		// check metrics
		metricNames, err := intake.GetMetricNames()
		require.NoError(t, err)
		assert.NotEmpty(t, metricNames)

		// check logs
		v.Env().Host.MustExecute("echo 'totoro' >> " + logFile)
		logs, err := intake.FilterLogs(logService)
		require.NoError(t, err)
		assert.NotEmpty(t, logs)

		// check traces
		teardownTraceGen := runUDSTraceGenerator(v.Env().Host, "test", "extratags")
		defer teardownTraceGen()
		traces, err := intake.GetTraces()
		require.NoError(t, err)
		assert.NotEmpty(t, traces)

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

// requireIntakeNotUsed checks that the given intake doesn't receive any payloads,
// after sending logs, flares, and traces.
func (v *multiFakeIntakeSuite) requireIntakeNotUsed(intake *fi.Client, intakeMaxWaitTime, intakeTick time.Duration) {
	checkFn := func(t *assert.CollectT) {
		// flush intake
		intake.FlushServerAndResetAggregators()

		// write a log
		v.Env().Host.MustExecute("echo 'totoro' >> " + logFile)

		// send a flare
		v.Env().Agent.Client.Flare(agentclient.WithArgs([]string{"--email", "e2e@test.com", "--send"}))

		// send traces
		teardownTraceGen := runUDSTraceGenerator(v.Env().Host, "test", "extratags")
		defer teardownTraceGen()

		// give time to the agent to flush to the intake
		v.T().Logf("waiting for the agent to flush to ensure the intake %s is not used", intake.URL())
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
