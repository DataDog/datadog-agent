// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbom

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
)

// windowsServer2025AMI is a Windows Server 2025 e2e image built before the
// November 2026 cumulative update, so its host always misses security updates.
const windowsServer2025AMI = "ami-0828f4241887f7d2d"

// hostWindowsAgentConfig turns on the host SBOM.
const hostWindowsAgentConfig = `sbom:
  enabled: true
  host:
    enabled: true
`

// findingsPageLimit is the most findings one page of the findings search
// returns.
const findingsPageLimit = 150

// kbPattern matches the name of a Windows update and captures its number.
var kbPattern = regexp.MustCompile(`^KB(\d+)$`)

type hostWindowsSuite struct {
	baseSuite[environments.Host]
}

// TestSBOMHostWindowsSuite runs the Agent with the host SBOM on Windows Server
// 2025 and checks the SBOM in fakeintake.
func TestSBOMHostWindowsSuite(t *testing.T) {
	e2e.Run(t, &hostWindowsSuite{}, e2e.WithProvisioner(awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithEC2InstanceOptions(scenec2.WithAMI(windowsServer2025AMI, e2eos.WindowsServer2025, e2eos.AMD64Arch)),
			scenec2.WithFakeIntakeOptions(fakeintake.WithRetentionPeriod(sbomHostRetentionPeriod)),
			scenec2.WithAgentOptions(agentparams.WithAgentConfig(hostWindowsAgentConfig)),
		),
	)))
}

func (s *hostWindowsSuite) SetupSuite() {
	s.baseSuite.SetupSuite()
	s.Fakeintake = s.Env().FakeIntake.Client()
}

// Test00HostSBOM checks that the host SBOM names the OS and lists the updates
// Windows reports as installed.
func (s *hostWindowsSuite) Test00HostSBOM() {
	ubr := strings.TrimSpace(s.Env().RemoteHost.MustExecute(`(Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion').UBR`))
	kbs := installedKBs(s.Env().RemoteHost)

	s.EventuallyWithTf(func(c *assert.CollectT) {
		bom := newestHostBOM(c, s.Fakeintake, time.Time{})
		var osComponent *cyclonedx_v1_4.Component
		var files []string
		for _, comp := range bom.GetComponents() {
			switch comp.GetType() {
			case cyclonedx_v1_4.Classification_CLASSIFICATION_OPERATING_SYSTEM:
				osComponent = comp
			case cyclonedx_v1_4.Classification_CLASSIFICATION_FILE:
				files = append(files, comp.GetName())
			}
		}
		require.NotNil(c, osComponent, "the host SBOM has no operating system component")
		assert.Equal(c, "windows", osComponent.GetName())
		assert.Equal(c, "10.0 Build 26100", osComponent.GetVersion())
		props := map[string]string{}
		for _, p := range osComponent.GetProperties() {
			props[p.GetName()] = p.GetValue()
		}
		assert.Truef(c, strings.HasPrefix(props["Platform"], "Microsoft Windows Server 2025"), "Platform is %q", props["Platform"])
		assert.Equal(c, "Server", props["Family"])
		assert.Equal(c, fmt.Sprintf("10.0.26100.%s Build 26100.%s", ubr, ubr), props["Build"])
		assert.Equal(c, "x86_64", props["Architecture"])
		slices.Sort(files)
		assert.Equal(c, kbs, slices.Compact(files), "the host SBOM lists other updates than Windows")
	}, 10*time.Minute, 30*time.Second, "no host SBOM described the host")
}

type hostWindowsVulnerabilitiesSuite struct {
	baseSuite[environments.Host]
}

// TestSBOMHostWindowsVulnerabilitiesSuite runs the Agent with the host SBOM on
// Windows Server 2025 and checks the vulnerabilities the backend of the e2e org
// matches. Its Agent ships to the e2e org directly, since the SBOMs fakeintake
// forwards never reach the vulnerability backend.
func TestSBOMHostWindowsVulnerabilitiesSuite(t *testing.T) {
	e2e.Run(t, &hostWindowsVulnerabilitiesSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake(
		awshost.WithRunOptions(
			scenec2.WithEC2InstanceOptions(
				scenec2.WithAMI(windowsServer2025AMI, e2eos.WindowsServer2025, e2eos.AMD64Arch),
				scenec2.WithInternetAccess(),
			),
			scenec2.WithAgentOptions(agentparams.WithAgentConfig(hostWindowsAgentConfig)),
		),
	)))
}

// Test00HostVulnerabilities checks that the backend reports the updates the
// host misses, and none of those it has installed.
func (s *hostWindowsVulnerabilitiesSuite) Test00HostVulnerabilities() {
	hostname := s.Env().Agent.Client.Hostname()
	installed := installedKBs(s.Env().RemoteHost)
	newestInstalled := 0
	for _, kb := range installed {
		if n, ok := kbNumber(kb); ok {
			newestInstalled = max(newestInstalled, n)
		}
	}

	client := newFindingsClient(s.T())
	filter := "@finding_type:host_and_container_vulnerability @status:open @origin:agent @host.name:" + hostname
	s.EventuallyWithTf(func(c *assert.CollectT) {
		_, err := client.search(filter)
		assert.NoError(c, err)
	}, 2*time.Minute, 10*time.Second, "the findings search of the e2e org fails")

	var byKB map[string]int
	s.EventuallyWithTf(func(c *assert.CollectT) {
		findings, err := client.search(filter)
		require.NoError(c, err)
		require.NotEmptyf(c, findings, "the backend reports no open vulnerability on %s yet", hostname)
		byKB = map[string]int{}
		newest := 0
		for _, f := range findings {
			attrs := f.GetAttributes().Attributes
			assert.Equalf(c, "10.0_build_26100", attribute(attrs, "host", "os", "version"), "OS version of finding %s", f.GetId())
			kb := attribute(attrs, "package", "version")
			n, ok := kbNumber(kb)
			if !assert.Truef(c, ok, "finding %s names %q, not a KB", f.GetId(), kb) {
				continue
			}
			assert.NotContainsf(c, installed, kb, "finding %s names %s, which the host has installed", f.GetId(), kb)
			newest = max(newest, n)
			byKB[kb]++
		}
		assert.Greaterf(c, newest, newestInstalled, "no finding names an update newer than KB%d, the newest installed", newestInstalled)
	}, 45*time.Minute, time.Minute, "the backend reported no Windows vulnerability on %s", hostname)
	s.T().Logf("open vulnerabilities on %s by KB: %v", hostname, byKB)
}

// installedKBs returns the sorted updates Win32_QuickFixEngineering lists on
// host, the source of the host SBOM.
func installedKBs(host *components.RemoteHost) []string {
	kbs := strings.Fields(host.MustExecute(`(Get-CimInstance Win32_QuickFixEngineering).HotFixID`))
	slices.Sort(kbs)
	return slices.Compact(kbs)
}

// kbNumber returns the number of the Windows update kb names.
func kbNumber(kb string) (int, bool) {
	m := kbPattern.FindStringSubmatch(kb)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// attribute returns the string at path in the attributes of a finding, or ""
// if there is none.
func attribute(attrs map[string]any, path ...string) string {
	var v any = attrs
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[k]
	}
	s, _ := v.(string)
	return s
}

// findingsClient searches the security findings of the e2e org.
type findingsClient struct {
	api *datadogV2.SecurityMonitoringApi
	ctx context.Context
}

// newFindingsClient returns a client with the keys of the e2e profile.
func newFindingsClient(t require.TestingT) *findingsClient {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	require.NoError(t, err)
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	require.NoError(t, err)
	ctx := context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: apiKey},
		"appKeyAuth": {Key: appKey},
	})
	return &findingsClient{
		api: datadogV2.NewSecurityMonitoringApi(datadog.NewAPIClient(datadog.NewConfiguration())),
		ctx: ctx,
	}
}

// search returns every finding filter selects. filter follows the log search
// syntax.
func (c *findingsClient) search(filter string) ([]datadogV2.SecurityFindingsData, error) {
	var findings []datadogV2.SecurityFindingsData
	page := datadogV2.SecurityFindingsSearchRequestPage{Limit: datadog.PtrInt64(findingsPageLimit)}
	for {
		req := datadogV2.SecurityFindingsSearchRequest{
			Data: &datadogV2.SecurityFindingsSearchRequestData{
				Attributes: &datadogV2.SecurityFindingsSearchRequestDataAttributes{
					Filter: &filter,
					Page:   &page,
				},
			},
		}
		resp, _, err := c.api.SearchSecurityFindings(c.ctx, req)
		if err != nil {
			return nil, fmt.Errorf("search findings %q: %w", filter, err)
		}
		findings = append(findings, resp.Data...)
		if len(resp.Data) == 0 || resp.Meta == nil || resp.Meta.Page == nil || resp.Meta.Page.GetAfter() == "" {
			return findings, nil
		}
		page.Cursor = resp.Meta.Page.After
	}
}
