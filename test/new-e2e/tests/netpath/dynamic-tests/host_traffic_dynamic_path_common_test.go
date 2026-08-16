// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkpathdynamictests

import (
	"embed"
	"fmt"
	"strings"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed fixtures/host_traffic_dns.py
var hostTrafficDNSFiles embed.FS

const (
	hostTrafficRemoteConfigDomain = "httpbin-rc.dynamic-netpath.test"
	hostTrafficDNSRemotePath      = "/tmp/host_traffic_dns.py"
	hostTrafficDNSLogPath         = "/tmp/host_traffic_dns.log"
	hostTrafficDNSPIDPath         = "/tmp/host_traffic_dns.pid"
	hostTrafficHTTPRootPath       = "/tmp/host_traffic_http_root"
	hostTrafficHTTPLogPath        = "/tmp/host_traffic_http.log"
	hostTrafficHTTPPIDPath        = "/tmp/host_traffic_http.pid"
	hostTrafficResolverBackupPath = "/tmp/host_traffic_resolv.conf.backup"
	hostTrafficResolverLinkPath   = "/tmp/host_traffic_resolv.conf.link"
	hostTrafficGeneratorLogPath   = "/tmp/host_traffic_dynamic_path_generator.log"
	hostTrafficGeneratorPIDPath   = "/tmp/host_traffic_dynamic_path_generator.pid"
)

type hostTrafficDynamicPathEnv struct {
	environments.Host
	HTTPBinHost *components.RemoteHost
}

type hostTrafficDynamicPathBaseSuite struct {
	e2e.BaseSuite[hostTrafficDynamicPathEnv]
}

func hostTrafficDynamicPathProvisioner(name, agentConfig, systemProbeConfig string) provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[hostTrafficDynamicPathEnv](name, func(ctx *pulumi.Context, env *hostTrafficDynamicPathEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		params := ec2.GetParams(
			ec2.WithName("hosttrafficdynamicpathvm"),
			ec2.WithAgentOptions(
				agentparams.WithAgentConfig(agentConfig),
				agentparams.WithSystemProbeConfig(systemProbeConfig),
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
		// port 80 by default. Stop and disable it so the suite's local Python HTTP
		// server can claim the port. `|| true` keeps this idempotent when it is absent.
		_, err = httpbinHost.OS.Runner().Command(
			"stop-apache2",
			&command.Args{
				Create: pulumi.String("systemctl disable --now apache2 || true"),
				Sudo:   true,
			},
		)
		return err
	}, nil)
}

func (s *hostTrafficDynamicPathBaseSuite) setupHostTraffic() {
	s.startHostTrafficHTTPServer()
	s.startHostTrafficDNSServer()
	s.assertHostTrafficServiceReady()
	s.assertHostTrafficServiceReachable()
	s.configureAgentResolver()
	s.assertHostTrafficDomainResolves()
}

func (s *hostTrafficDynamicPathBaseSuite) tearDownHostTraffic() {
	s.stopHostTrafficGenerator()
	s.restoreAgentResolver()
	s.stopHostTrafficDNSServer()
	s.stopHostTrafficHTTPServer()
}

func (s *hostTrafficDynamicPathBaseSuite) AfterTest(suiteName, testName string) {
	if s.T().Failed() {
		s.logRemoteFile(s.Env().HTTPBinHost, hostTrafficDNSLogPath)
		s.logRemoteFile(s.Env().HTTPBinHost, hostTrafficHTTPLogPath)
		s.logRemoteFile(s.Env().RemoteHost, hostTrafficGeneratorLogPath)
	}
	s.BaseSuite.AfterTest(suiteName, testName)
}

func (s *hostTrafficDynamicPathBaseSuite) startHostTrafficHTTPServer() {
	host := s.Env().HTTPBinHost
	startCommand := fmt.Sprintf(
		"mkdir -p %s; cd %s; printf 'ok\\n' >index.html; nohup python3 -m http.server 80 --bind 0.0.0.0 >%s 2>&1 & echo $! >%s",
		shellQuote(hostTrafficHTTPRootPath),
		shellQuote(hostTrafficHTTPRootPath),
		shellQuote(hostTrafficHTTPLogPath),
		shellQuote(hostTrafficHTTPPIDPath),
	)
	host.MustExecute(fmt.Sprintf(`if [ -f %s ]; then sudo kill "$(sudo cat %s)" || true; fi
sudo rm -f %s %s
sudo sh -c %s
sleep 1
sudo kill -0 "$(sudo cat %s)"
`,
		shellQuote(hostTrafficHTTPPIDPath),
		shellQuote(hostTrafficHTTPPIDPath),
		shellQuote(hostTrafficHTTPPIDPath),
		shellQuote(hostTrafficHTTPLogPath),
		shellQuote(startCommand),
		shellQuote(hostTrafficHTTPPIDPath),
	))
}

func (s *hostTrafficDynamicPathBaseSuite) stopHostTrafficHTTPServer() {
	if s.Env().HTTPBinHost == nil {
		return
	}
	_, err := s.Env().HTTPBinHost.Execute(fmt.Sprintf(`if [ -f %s ]; then sudo kill "$(sudo cat %s)" || true; fi`, shellQuote(hostTrafficHTTPPIDPath), shellQuote(hostTrafficHTTPPIDPath)))
	if err != nil {
		s.T().Logf("failed to stop host traffic HTTP server: %v", err)
	}
}

func (s *hostTrafficDynamicPathBaseSuite) assertHostTrafficServiceReady() {
	s.Env().HTTPBinHost.MustExecute(`i=0; while [ "$i" -lt 30 ]; do
  python3 -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1/", timeout=5).read()' && exit 0
  sleep 2
  i=$((i+1))
done
exit 1`)
}

func (s *hostTrafficDynamicPathBaseSuite) assertHostTrafficServiceReachable() {
	url := hostTrafficURL(s.Env().HTTPBinHost.Address)
	script := fmt.Sprintf(`import time, urllib.request
url = %q
for attempt in range(3):
    try:
        urllib.request.urlopen(url, timeout=5).read()
        break
    except Exception:
        if attempt == 2:
            raise
        time.sleep(2)
`, url)
	s.Env().RemoteHost.MustExecute("python3 -c " + shellQuote(script))
}

func (s *hostTrafficDynamicPathBaseSuite) startHostTrafficDNSServer() {
	httpbinHost := s.Env().HTTPBinHost
	httpbinHost.CopyFileFromFS(hostTrafficDNSFiles, "fixtures/host_traffic_dns.py", hostTrafficDNSRemotePath)
	httpbinHost.MustExecute("sudo chmod 0755 " + shellQuote(hostTrafficDNSRemotePath))

	upstream := strings.TrimSpace(httpbinHost.MustExecute("awk '/^nameserver / && $2 ~ /^[0-9.]+$/ {print $2; exit}' /etc/resolv.conf"))
	require.NotEmpty(s.T(), upstream, "could not find an IPv4 DNS upstream on the HTTPBin host")

	startCommand := fmt.Sprintf(
		"nohup python3 %s %s %s %s %s >%s 2>&1 & echo $! >%s",
		shellQuote(hostTrafficDNSRemotePath),
		shellQuote(httpbinHost.Address),
		shellQuote(hostTrafficRemoteConfigDomain),
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

func (s *hostTrafficDynamicPathBaseSuite) stopHostTrafficDNSServer() {
	if s.Env().HTTPBinHost == nil {
		return
	}
	_, err := s.Env().HTTPBinHost.Execute(fmt.Sprintf(`if [ -f %s ]; then sudo kill "$(sudo cat %s)" || true; fi`, shellQuote(hostTrafficDNSPIDPath), shellQuote(hostTrafficDNSPIDPath)))
	if err != nil {
		s.T().Logf("failed to stop host traffic DNS server: %v", err)
	}
}

func (s *hostTrafficDynamicPathBaseSuite) configureAgentResolver() {
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

func (s *hostTrafficDynamicPathBaseSuite) restoreAgentResolver() {
	if s.Env().RemoteHost == nil {
		return
	}
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`set +e
if [ -f %s ]; then
  if [ -s %s ]; then
    target="$(cat %s)"
    sudo rm -f /etc/resolv.conf
    sudo ln -s "$target" /etc/resolv.conf
  else
    sudo cp %s /etc/resolv.conf
  fi
fi
`, shellQuote(hostTrafficResolverBackupPath), shellQuote(hostTrafficResolverLinkPath), shellQuote(hostTrafficResolverLinkPath), shellQuote(hostTrafficResolverBackupPath)))
	if err != nil {
		s.T().Logf("failed to restore resolver: %v", err)
	}
}

func (s *hostTrafficDynamicPathBaseSuite) assertHostTrafficDomainResolves() {
	output := s.Env().RemoteHost.MustExecute("getent ahostsv4 " + shellQuote(hostTrafficRemoteConfigDomain))
	require.Contains(s.T(), output, s.Env().HTTPBinHost.Address)
}

func (s *hostTrafficDynamicPathBaseSuite) startHostTrafficGenerator(duration time.Duration) {
	seconds := int(duration.Seconds())
	trafficScript := fmt.Sprintf(
		"import time, urllib.request\nurl = %q\nend = time.monotonic() + %d\nwhile time.monotonic() < end:\n    try:\n        urllib.request.urlopen(url, timeout=5).read()\n    except Exception:\n        pass\n    time.sleep(2)\n",
		hostTrafficURL(hostTrafficRemoteConfigDomain),
		seconds,
	)
	s.Env().RemoteHost.MustExecute(fmt.Sprintf("nohup python3 -c %s >%s 2>&1 & echo $! >%s",
		shellQuote(trafficScript),
		shellQuote(hostTrafficGeneratorLogPath),
		shellQuote(hostTrafficGeneratorPIDPath),
	))
}

func (s *hostTrafficDynamicPathBaseSuite) stopHostTrafficGenerator() {
	if s.Env().RemoteHost == nil {
		return
	}
	_, err := s.Env().RemoteHost.Execute(fmt.Sprintf(`if [ -f %s ]; then kill "$(cat %s)" || true; fi`, shellQuote(hostTrafficGeneratorPIDPath), shellQuote(hostTrafficGeneratorPIDPath)))
	if err != nil {
		s.T().Logf("failed to stop host traffic generator: %v", err)
	}
}

func (s *hostTrafficDynamicPathBaseSuite) logRemoteFile(host *components.RemoteHost, path string) {
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

func assertHostTrafficNetworkPath(c *assert.CollectT, netpaths []*aggregator.Netpath, expectedProfile payload.DynamicTestProfile, description string) *aggregator.Netpath {
	match := findHostTrafficNetworkPath(netpaths, hostTrafficRemoteConfigDomain)
	require.NotNil(c, match, "no %s host-traffic network path event matched %s:80", description, hostTrafficRemoteConfigDomain)

	assert.Equal(c, payload.PathOriginNetworkTraffic, match.Origin)
	assert.Equal(c, payload.SourceProductNetworkPath, match.SourceProduct)
	assert.Equal(c, payload.TestRunTypeDynamic, match.TestRunType)
	assert.Equal(c, expectedProfile, match.DynamicTestProfile)
	assert.Equal(c, payload.CollectorTypeAgent, match.CollectorType)
	assert.Equal(c, payload.ProtocolTCP, match.Protocol)
	assert.Equal(c, hostTrafficRemoteConfigDomain, match.Destination.Hostname)
	assert.Equal(c, uint16(80), match.Destination.Port)
	require.NotEmpty(c, match.Traceroute.Runs, "matched network path has no traceroute runs")
	assert.True(c, hasTracerouteDestinationIP(match), "matched network path has no traceroute destination IP")
	return match
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
