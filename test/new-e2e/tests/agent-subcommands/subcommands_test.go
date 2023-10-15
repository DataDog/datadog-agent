// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentsubcommands

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/cenkalti/backoff"
	"github.com/stretchr/testify/assert"
)

type subcommandSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func TestSubcommandSuite(t *testing.T) {
	e2e.Run(t, &subcommandSuite{}, e2e.FakeIntakeStackDef())
}

// section contains the content status of a specific section (e.g. Forwarder)
type section struct {
	name    string
	content string
}

// getStatusComponentContent returns the content of the `sectionName` section if it exists, or an error otherwise
func getStatusComponentContent(statusOutput string, sectionName string) (*section, error) {
	// A status section is composed of:
	// * a name (e.g. 'Forwarder')
	// * followed by lines whose first character is not a blank (e.g. '==========')
	// * and then lines starting with blank characters (which is basically the content of the section)
	linesStartingWithNonWhiteCharRegex := `([^[:blank:]]+\n)+`                              // new sections aren't indented
	linesStartingWithWhiteCharRegex := `(?P<sectionContent>(?m:^[[:blank:]]+.*\n|^\r?\n)+)` // either match starting with blank or match empty line (\r\n and \n)
	regexTemplate := fmt.Sprintf("%v\n", sectionName) + linesStartingWithNonWhiteCharRegex + linesStartingWithWhiteCharRegex

	re := regexp.MustCompile(regexTemplate)
	matches := re.FindStringSubmatch(statusOutput)

	if len(matches) == 0 {
		return nil, fmt.Errorf("regexp: no matches for %s status section", sectionName)
	}

	settingsIndex := re.SubexpIndex("sectionContent")

	return &section{
		name:    sectionName,
		content: matches[settingsIndex],
	}, nil
}

// expectedSection contains the information we want to verify about a section content
type expectedSection struct {
	name             string
	shouldBePresent  bool
	shouldContain    []string
	shouldNotContain []string
}

func (v *subcommandSuite) TestDefaultInstallStatus() {

	v.UpdateEnv(e2e.FakeIntakeStackDef())

	metadata := client.NewEC2Metadata(v.Env().VM)
	resourceID := metadata.Get("instance-id")

	expectedSections := []expectedSection{
		{
			name:             `Agent \(.*\)`, // TODO: verify that the right version is output
			shouldBePresent:  true,
			shouldContain:    []string{fmt.Sprintf("hostname: %v", resourceID), "hostname provider: aws"},
			shouldNotContain: []string{"FIPS proxy"},
		},
		{
			name:            "Aggregator",
			shouldBePresent: true,
		},
		{
			name:             "APM Agent",
			shouldBePresent:  true,
			shouldContain:    []string{"Status: Running"},
			shouldNotContain: []string{"Error"},
		},
		{
			name:            "Autodiscovery",
			shouldBePresent: false,
		},
		{
			name:             "Collector",
			shouldBePresent:  true,
			shouldContain:    []string{"Instance ID: cpu [OK]"},
			shouldNotContain: []string{"Errors"},
		},
		{
			name:            "Compliance",
			shouldBePresent: false,
		},
		{
			name:            "Custom Metrics Server",
			shouldBePresent: false,
		},
		{
			name:            "Datadog Cluster Agent",
			shouldBePresent: false,
		},
		{
			name:            "DogStatsD",
			shouldBePresent: true,
		},
		{
			name:             "Endpoints",
			shouldBePresent:  true,
			shouldNotContain: []string{"No endpoints information. The agent may be misconfigured."},
		},
		{
			name:             "Forwarder",
			shouldBePresent:  true,
			shouldNotContain: []string{"API Keys errors"},
		},
		{
			name:            "JMXFetch",
			shouldBePresent: true,
			shouldContain:   []string{"no checks"},
		},
		{
			name:            "Logs Agent",
			shouldBePresent: true,
			shouldContain:   []string{"Logs Agent is not running"},
		},
		{
			name:            "Metadata Mapper",
			shouldBePresent: false,
		},
		{
			name:            "Orchestrator Explorer",
			shouldBePresent: false,
		},
		{
			name:            "OTLP",
			shouldBePresent: true,
			shouldContain:   []string{"Status: Not enabled"},
		},
		{
			name:             "Process Agent",
			shouldBePresent:  true,
			shouldNotContain: []string{"Status: Not running or unreachable"},
		},
		{
			name:            "Remote Configuration",
			shouldBePresent: true,
		},
		{
			name:            "Runtime Security",
			shouldBePresent: false,
		},
		{
			name:            "SNMP Traps",
			shouldBePresent: false,
		},
		{
			name:            "System Probe",
			shouldBePresent: false,
		},
		{
			// XXX: this test is expected to fail until 7.48 as a known status render errors has been fixed in #18123
			name:            "Status render errors",
			shouldBePresent: false,
		},
	}

	status := v.Env().Agent.Status()

	for _, section := range expectedSections {
		verifySectionContent(v.T(), status.Content, section)
	}
}

func (v *subcommandSuite) TestFIPSProxyStatus() {

	v.UpdateEnv(e2e.FakeIntakeStackDef(e2e.WithAgentParams(agentparams.WithAgentConfig("fips.enabled: true"))))
	expectedSection := expectedSection{
		name:            `Agent \(.*\)`,
		shouldBePresent: true,
		shouldContain:   []string{"FIPS proxy"},
	}
	status := v.Env().Agent.Status()
	verifySectionContent(v.T(), status.Content, expectedSection)
}

// verifySectionContent verifies that a specific status section behaves as expected (is correctly present or not, contains specific strings or not)
func verifySectionContent(t *testing.T, statusOutput string, section expectedSection) {

	sectionContent, err := getStatusComponentContent(statusOutput, section.name)

	if section.shouldBePresent {
		if assert.NoError(t, err, "Section %v was expected in the status output, but was not found", section.name) {
			for _, expectedContent := range section.shouldContain {
				assert.Contains(t, sectionContent.content, expectedContent)
			}

			for _, unexpectedContent := range section.shouldNotContain {
				assert.NotContains(t, sectionContent.content, unexpectedContent)
			}
		}
	} else {
		assert.Error(t, err, "Section %v should not be present in the status output, but was found with the following content: %v", section.name, sectionContent)
	}
}

func (v *subcommandSuite) TestDefaultInstallHealthy() {
	v.UpdateEnv(e2e.FakeIntakeStackDef())
	interval := 1 * time.Second

	var output string
	var err error
	err = backoff.Retry(func() error {
		output, err = v.Env().Agent.Health()
		if err != nil {
			return err
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(interval), uint64(15)))

	assert.NoError(v.T(), err)
	assert.Contains(v.T(), output, "Agent health: PASS")
}
