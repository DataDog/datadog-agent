// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpathdynamictests

import (
	"embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed config/host_traffic_dynamic_path.yaml
var hostTrafficDynamicPathAgentConfig string

//go:embed config/host_traffic_system_probe.yaml
var hostTrafficSystemProbeConfig string

//go:embed fixtures/host_traffic_dns.py
var hostTrafficDNSFiles embed.FS

const (
	hostTrafficLocalDomain        = "httpbin.dynamic-netpath.test"
	hostTrafficRemoteConfigDomain = "httpbin-rc.dynamic-netpath.test"
	hostTrafficDNSRemotePath      = "/tmp/host_traffic_dns.py"
	hostTrafficDNSLogPath         = "/tmp/host_traffic_dns.log"
	hostTrafficDNSPIDPath         = "/tmp/host_traffic_dns.pid"
	hostTrafficResolverBackupPath = "/tmp/host_traffic_resolv.conf.backup"
	hostTrafficResolverLinkPath   = "/tmp/host_traffic_resolv.conf.link"
	hostTrafficCurlLogPath        = "/tmp/host_traffic_dynamic_path_curl.log"
	hostTrafficCurlPIDPath        = "/tmp/host_traffic_dynamic_path_curl.pid"
	hostTrafficRCProduct          = "NETWORK_PATH"
	hostTrafficRCConfigID         = "test-config-dynamic-host-traffic"
	hostTrafficRCConfigName       = "config"
	hostTrafficHTTPBinComposeYAML = `version: '3.9'
services:
  httpbin:
    pid: host
    privileged: true
    ports:
    - 80:8080/tcp
    image: ghcr.io/datadog/apps-go-httpbin:{APPS_VERSION}
    container_name: httpbin
    volumes: []
    environment: {}
`
)

var hostTrafficDynamicRCConfig = []byte(`{
  "type": "dynamic",
  "test_config_id": "dynamic-host-traffic",
  "config": {
    "filters": [
      {
        "type": "include",
        "match_domain": "httpbin-rc.dynamic-netpath.test",
        "match_domain_strategy": "wildcard"
      }
    ]
  }
}`)

type hostTrafficDynamicPathEnv struct {
	environments.Host
	HTTPBinHost *components.RemoteHost
}

type hostTrafficDynamicPathSuite struct {
	e2e.BaseSuite[hostTrafficDynamicPathEnv]
	remoteConfigAdded bool
}

// TestHostTrafficDynamicPathSuite runs Network Path Dynamic Tests backed by host NPM traffic.
func TestHostTrafficDynamicPathSuite(t *testing.T) {
	e2e.Run(t, &hostTrafficDynamicPathSuite{}, e2e.WithProvisioner(hostTrafficDynamicPathProvisioner()))
}

func hostTrafficDynamicPathProvisioner() provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[hostTrafficDynamicPathEnv]("hostTrafficDynamicPath", func(ctx *pulumi.Context, env *hostTrafficDynamicPathEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		params := ec2.GetParams(
			ec2.WithName("hosttrafficdynamicpathvm"),
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(hostTrafficDynamicPathAgentConfig),
				agentparams.WithSystemProbeConfig(hostTrafficSystemProbeConfig),
			),
		)
		if err := ec2.Run(ctx, awsEnv, env, params); err != nil {
			return err
		}

		httpbinHost, err := ec2.NewVM(awsEnv, "hosttraffichttpbinvm")
		if err != nil {
			return err
		}
		if err := httpbinHost.Export(ctx, &env.HTTPBinHost.HostOutput); err != nil {
			return err
		}

		// The Ubuntu e2e AMI installs apache2 (via the php meta-package) which binds to
		// port 80 by default. Stop and disable it so the httpbin container below can
		// claim the port during docker-compose up. `|| true` keeps this idempotent on
		// hosts where apache2 is absent.
		stopApache, err := httpbinHost.OS.Runner().Command(
			"stop-apache2",
			&command.Args{
				Create: pulumi.String("systemctl disable --now apache2 || true"),
				Sudo:   true,
			},
		)
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewAWSManager(&awsEnv, httpbinHost, utils.PulumiDependsOn(stopApache))
		if err != nil {
			return err
		}

		_, err = dockerManager.ComposeStrUp("httpbin", []docker.ComposeInlineManifest{hostTrafficHTTPBinCompose()}, pulumi.StringMap{})
		return err
	}, nil)
}

func hostTrafficHTTPBinCompose() docker.ComposeInlineManifest {
	return docker.ComposeInlineManifest{
		Name:    "httpbin",
		Content: pulumi.String(strings.ReplaceAll(hostTrafficHTTPBinComposeYAML, "{APPS_VERSION}", apps.Version)),
	}
}

func (s *hostTrafficDynamicPathSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()

	s.ensureCurlInstalled()
	s.startHostTrafficDNSServer()
	s.configureAgentResolver()
	s.assertHostTrafficDomainResolves()

	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		stats, err := fakeintake.RCStats()
		assert.NoError(c, err)
		assert.NotZero(c, stats.Polls, "agent did not poll fakeintake Remote Config")
	}, 2*time.Minute, 5*time.Second)
	require.NoError(s.T(), fakeintake.RCAddConfig("", hostTrafficRCProduct, hostTrafficRCConfigID, hostTrafficRCConfigName, hostTrafficDynamicRCConfig))
	s.remoteConfigAdded = true

	require.NoError(s.T(), fakeintake.FlushServerAndResetAggregators())
}

func (s *hostTrafficDynamicPathSuite) TearDownSuite() {
	s.deleteHostTrafficRemoteConfig()
	s.stopHostTrafficGenerator()
	s.restoreAgentResolver()
	s.stopHostTrafficDNSServer()
	s.BaseSuite.TearDownSuite()
}

func (s *hostTrafficDynamicPathSuite) AfterTest(suiteName, testName string) {
	if s.T().Failed() {
		s.logRemoteFile(s.Env().HTTPBinHost, hostTrafficDNSLogPath)
		s.logRemoteFile(s.Env().RemoteHost, hostTrafficCurlLogPath)
	}
	s.BaseSuite.AfterTest(suiteName, testName)
}

func (s *hostTrafficDynamicPathSuite) TestHostTrafficDynamicNetworkPath() {
	fakeintake := s.Env().FakeIntake.Client()
	s.startHostTrafficGenerator(4 * time.Minute)

	var remoteConfigMatch *aggregator.Netpath
	s.EventuallyWithT(func(c *assert.CollectT) {
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.schedule.pathtest_count")
		assertMetricPresent(c, fakeintake, "datadog.network_path.collector.flush.pathtest_count")

		netpaths, err := fakeintake.GetLatestNetpathEvents()
		require.NoError(c, err)
		require.NotEmpty(c, netpaths, "no network path events")

		for _, domain := range []string{hostTrafficLocalDomain, hostTrafficRemoteConfigDomain} {
			match := findHostTrafficNetworkPath(netpaths, domain)
			require.NotNil(c, match, "no host-traffic network path event matched %s:80", domain)

			assert.Equal(c, payload.PathOriginNetworkTraffic, match.Origin)
			assert.Equal(c, payload.SourceProductNetworkPath, match.SourceProduct)
			assert.Equal(c, payload.TestRunTypeDynamic, match.TestRunType)
			assert.Equal(c, payload.CollectorTypeAgent, match.CollectorType)
			assert.Equal(c, payload.ProtocolTCP, match.Protocol)
			assert.Equal(c, domain, match.Destination.Hostname)
			assert.Equal(c, uint16(80), match.Destination.Port)
			require.NotEmpty(c, match.Traceroute.Runs, "matched network path has no traceroute runs")
			assert.True(c, hasTracerouteDestinationIP(match), "matched network path has no traceroute destination IP")

			if domain == hostTrafficRemoteConfigDomain {
				remoteConfigMatch = match
			}
		}
	}, 5*time.Minute, 10*time.Second)

	if remoteConfigMatch != nil {
		s.T().Logf("matched RC host traffic dynamic path destination=%s:%d test_run_id=%s",
			remoteConfigMatch.Destination.Hostname,
			remoteConfigMatch.Destination.Port,
			remoteConfigMatch.TestRunID,
		)
	}
}

func (s *hostTrafficDynamicPathSuite) deleteHostTrafficRemoteConfig() {
	if !s.remoteConfigAdded {
		return
	}
	fakeintake := s.Env().FakeIntake.Client()
	configs, err := fakeintake.RCListConfigs()
	if err != nil {
		s.T().Logf("failed to list Remote Config entries during cleanup: %v", err)
		return
	}
	for _, config := range configs {
		if config.Product != hostTrafficRCProduct || config.ConfigID != hostTrafficRCConfigID || config.ConfigName != hostTrafficRCConfigName {
			continue
		}
		key := fmt.Sprintf("%s/%s/%s/%s", config.OrgID, config.Product, config.ConfigID, config.ConfigName)
		if err := fakeintake.RCDeleteConfig(key); err != nil {
			s.T().Logf("failed to delete Remote Config entry during cleanup: %v", err)
		} else {
			s.remoteConfigAdded = false
		}
		return
	}
}

func (s *hostTrafficDynamicPathSuite) ensureCurlInstalled() {
	s.Env().RemoteHost.MustExecute("if ! command -v curl >/dev/null 2>&1; then sudo apt-get update && sudo apt-get install -y curl; fi")
}

func (s *hostTrafficDynamicPathSuite) startHostTrafficDNSServer() {
	httpbinHost := s.Env().HTTPBinHost
	httpbinHost.CopyFileFromFS(hostTrafficDNSFiles, "fixtures/host_traffic_dns.py", hostTrafficDNSRemotePath)
	httpbinHost.MustExecute("sudo chmod 0755 " + shellQuote(hostTrafficDNSRemotePath))

	upstream := strings.TrimSpace(httpbinHost.MustExecute("awk '/^nameserver / && $2 ~ /^[0-9.]+$/ {print $2; exit}' /etc/resolv.conf"))
	require.NotEmpty(s.T(), upstream, "could not find an IPv4 DNS upstream on the HTTPBin host")

	startCommand := fmt.Sprintf(
		"nohup python3 %s %s %s %s %s >%s 2>&1 & echo $! >%s",
		shellQuote(hostTrafficDNSRemotePath),
		shellQuote(httpbinHost.Address),
		shellQuote(strings.Join([]string{hostTrafficLocalDomain, hostTrafficRemoteConfigDomain}, ",")),
		shellQuote(httpbinHost.Address),
		shellQuote(upstream),
		shellQuote(hostTrafficDNSLogPath),
		shellQuote(hostTrafficDNSPIDPath),
	)
	httpbinHost.MustExecute(fmt.Sprintf(`if [ -f %s ]; then sudo kill "$(sudo cat %s)" || true; fi
sudo rm -f %s %s
sudo sh -c %s
sleep 1
sudo kill -0 "$(sudo cat %s)"
`,
		shellQuote(hostTrafficDNSPIDPath),
		shellQuote(hostTrafficDNSPIDPath),
		shellQuote(hostTrafficDNSPIDPath),
		shellQuote(hostTrafficDNSLogPath),
		shellQuote(startCommand),
		shellQuote(hostTrafficDNSPIDPath),
	))
}

func (s *hostTrafficDynamicPathSuite) stopHostTrafficDNSServer() {
	if s.Env().HTTPBinHost == nil {
		return
	}
	_, err := s.Env().HTTPBinHost.Execute(fmt.Sprintf(`if [ -f %s ]; then sudo kill "$(sudo cat %s)" || true; fi`, shellQuote(hostTrafficDNSPIDPath), shellQuote(hostTrafficDNSPIDPath)))
	if err != nil {
		s.T().Logf("failed to stop host traffic DNS server: %v", err)
	}
}

func (s *hostTrafficDynamicPathSuite) configureAgentResolver() {
	dnsIP := s.Env().HTTPBinHost.Address
	s.Env().RemoteHost.MustExecute(fmt.Sprintf(`set -eu
if [ ! -e %s ]; then
  if [ -L /etc/resolv.conf ]; then readlink /etc/resolv.conf | sudo tee %s >/dev/null; fi
  sudo cp -L /etc/resolv.conf %s
fi
sudo rm -f /etc/resolv.conf
printf 'nameserver %s\noptions timeout:1 attempts:2\n' | sudo tee /etc/resolv.conf >/dev/null
`, shellQuote(hostTrafficResolverBackupPath), shellQuote(hostTrafficResolverLinkPath), shellQuote(hostTrafficResolverBackupPath), dnsIP))
}

func (s *hostTrafficDynamicPathSuite) restoreAgentResolver() {
	if s.Env().RemoteHost == nil {
		return
	}
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`set +e
if [ -f %s ]; then
  if [ -s %s ]; then
    target="$(cat %s)"
    sudo rm -f /etc/resolv.conf
    if [ -e "$target" ]; then
      sudo ln -s "$target" /etc/resolv.conf
    else
      sudo cp %s /etc/resolv.conf
    fi
  else
    sudo cp %s /etc/resolv.conf
  fi
fi
`, shellQuote(hostTrafficResolverBackupPath), shellQuote(hostTrafficResolverLinkPath), shellQuote(hostTrafficResolverLinkPath), shellQuote(hostTrafficResolverBackupPath), shellQuote(hostTrafficResolverBackupPath)))
	if err != nil {
		s.T().Logf("failed to restore resolver: %v", err)
	}
}

func (s *hostTrafficDynamicPathSuite) assertHostTrafficDomainResolves() {
	for _, domain := range []string{hostTrafficLocalDomain, hostTrafficRemoteConfigDomain} {
		output := s.Env().RemoteHost.MustExecute("getent ahostsv4 " + shellQuote(domain))
		require.Contains(s.T(), output, s.Env().HTTPBinHost.Address)

		s.Env().RemoteHost.MustExecute(fmt.Sprintf("curl -4 -fsS --retry 3 --max-time 5 %s >/dev/null", shellQuote(hostTrafficURL(domain))))
	}
}

func (s *hostTrafficDynamicPathSuite) startHostTrafficGenerator(duration time.Duration) {
	seconds := int(duration.Seconds())
	trafficCommand := fmt.Sprintf(
		"i=0; while [ \"$i\" -lt %d ]; do curl -4 -fsS --max-time 5 %s >/dev/null || true; curl -4 -fsS --max-time 5 %s >/dev/null || true; sleep 2; i=$((i+2)); done",
		seconds,
		shellQuote(hostTrafficURL(hostTrafficLocalDomain)),
		shellQuote(hostTrafficURL(hostTrafficRemoteConfigDomain)),
	)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("nohup sh -c %s >%s 2>&1 & echo $! >%s",
		shellQuote(trafficCommand),
		shellQuote(hostTrafficCurlLogPath),
		shellQuote(hostTrafficCurlPIDPath),
	))
}

func (s *hostTrafficDynamicPathSuite) stopHostTrafficGenerator() {
	if s.Env().RemoteHost == nil {
		return
	}
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`if [ -f %s ]; then kill "$(cat %s)" || true; fi`, shellQuote(hostTrafficCurlPIDPath), shellQuote(hostTrafficCurlPIDPath)))
	if err != nil {
		s.T().Logf("failed to stop host traffic generator: %v", err)
	}
}

func (s *hostTrafficDynamicPathSuite) logRemoteFile(host *components.RemoteHost, path string) {
	if host == nil {
		return
	}
	output, err := host.Execute(fmt.Sprintf("if [ -f %s ]; then sudo tail -n 200 %s; fi", shellQuote(path), shellQuote(path)))
	if err != nil {
		s.T().Logf("failed to read %s: %v", path, err)
		return
	}
	if strings.TrimSpace(output) != "" {
		s.T().Logf("%s:\n%s", path, output)
	}
}

func findHostTrafficNetworkPath(netpaths []*aggregator.Netpath, domain string) *aggregator.Netpath {
	for _, np := range netpaths {
		if np == nil {
			continue
		}
		if np.Origin == payload.PathOriginNetworkTraffic &&
			np.Protocol == payload.ProtocolTCP &&
			np.Destination.Hostname == domain &&
			np.Destination.Port == 80 {
			return np
		}
	}
	return nil
}

func hasTracerouteDestinationIP(np *aggregator.Netpath) bool {
	for _, run := range np.Traceroute.Runs {
		if len(run.Destination.IPAddress) > 0 {
			return true
		}
	}
	return false
}

func hostTrafficURL(domain string) string {
	return "http://" + domain + "/"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
