// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

const securityAgent = "/opt/datadog-agent/embedded/bin/security-agent"

const rulePrefix = "xccdf_org.ssgproject.content_rule_"

// The security-agent evaluates host benchmarks, so the config goes in security-agent.yaml.
const hostBenchmarksConfig = `compliance_config:
  enabled: true
  host_benchmarks:
    enabled: true
`

// event is the part of a compliance CheckEvent the assertions need.
type event struct {
	FrameworkID  string `json:"agent_framework_id"`
	RuleID       string `json:"agent_rule_id"`
	Result       string `json:"result"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

// band is an inclusive proportion range for one result.
type band struct{ min, max float64 }

// probe forces one rule to fail then pass, exercising one OpenSCAP probe.
type probe struct {
	name   string
	rule   string
	broken string // shell that makes the host fail the rule
	fixed  string // shell that makes the host pass the rule
}

// distro holds everything that varies between operating systems.
type distro struct {
	name        string
	os          os.Descriptor
	containerOS *os.Descriptor // container-variant VM OS if it must differ from os (docker pre-baked AMI); nil = os
	frameworkID string
	family      family // selects the package manager and service names for probe builders
	minRules    int
	bands       map[string]band
	onlyProbes  []string // nil = all applicable probes; else restrict to these names
	latestAMI   bool     // resolve the AMI by search rather than a pinned one (AlmaLinux)
}

// Bands bracket the observed host and container distributions with a ~7 point margin
// (10 for AlmaLinux, whose latest AMI drifts), tight enough to catch a real distribution
// regression while tolerating the small host/container delta. Actuals are logged, and the
// gating golden (host_benchmarks_diag_nix_test.go) catches the per-rule regressions the
// bands are too coarse to see.
var rhel10Bands = map[string]band{
	"passed":  {0.44, 0.60},
	"failed":  {0.30, 0.45},
	"skipped": {0.04, 0.18},
}

var ubuntu2404Bands = map[string]band{
	"passed":  {0.48, 0.63},
	"failed":  {0.19, 0.35},
	"skipped": {0.09, 0.23},
}

// almalinux9 resolves the latest AMI rather than a pinned one, so its distribution can
// drift with OS updates. Its band is wider (~10 points) and its golden stays informational.
var almalinux9Bands = map[string]band{
	"passed":  {0.38, 0.58},
	"failed":  {0.29, 0.49},
	"skipped": {0.02, 0.23},
}

// al2023 skips most rules because ssg-al2023 gates them on a missing kernel-core package
// at the pinned policy version, so the skip fraction is large and set by the content, not
// the host. These bands track that content-limited distribution. See distroAmazonLinux2023
// for the root cause and the follow-up (re-harvest when the fix ships).
var al2023Bands = map[string]band{
	"passed":  {0.17, 0.32},
	"failed":  {0.00, 0.10},
	"skipped": {0.65, 0.80},
}

var rhel8Bands = map[string]band{
	"passed":  {0.46, 0.60},
	"failed":  {0.27, 0.41},
	"skipped": {0.05, 0.20},
}

var rhel9Bands = map[string]band{
	"passed":  {0.44, 0.59},
	"failed":  {0.29, 0.44},
	"skipped": {0.05, 0.20},
}

var distroRHEL10 = distro{
	name:        "rhel10",
	os:          os.RedHat10,
	frameworkID: "cis-rhel10",
	family:      rhel,
	minRules:    230,
	bands:       rhel10Bands,
}

var distroUbuntu2404 = distro{
	name: "ubuntu2404",
	os:   os.Ubuntu2404,
	// The container VM needs Docker pre-baked: ec2docker no longer runtime-installs it
	// for Ubuntu, and the plain 24.04 AMI lacks it. The -e2e AMI is still Ubuntu 24.04.
	containerOS: &os.Ubuntu2404E2E,
	frameworkID: "cis-ubuntu2404",
	family:      debian,
	minRules:    320,
	bands:       ubuntu2404Bands,
}

var distroAmazonLinux2023 = distro{
	name:        "al2023",
	os:          os.AmazonLinux2023,
	frameworkID: "cis-al2023",
	family:      rhel,
	minRules:    200,
	bands:       al2023Bands,
	// At the pinned SECURITY_AGENT_POLICIES_VERSION, ssg-al2023's system_with_kernel
	// platform gates most rules on the kernel-core package. AL2023 ships kernel, not
	// kernel-core, so those rules are not applicable and skip regardless of host state.
	// The ComplianceAsCode fix (system_with_kernel matching AL2023's kernel package) is
	// not yet in a released version (latest is ComplianceAsCode v0.1.81). Until a release
	// with it is pulled into the pinned policy, exercise only the package probe (the
	// others would land on skipped rules and fail the must-evaluate check) and keep the
	// wide al2023Bands. Once it lands, drop onlyProbes for the full probe set, restore
	// normal bands, re-harvest the golden baseline, and raise minRules.
	onlyProbes: []string{"package"},
}

var distroAlmaLinux9 = distro{
	name:        "almalinux9",
	os:          os.AlmaLinux9,
	frameworkID: "cis-almalinux9",
	family:      rhel,
	minRules:    230,
	bands:       almalinux9Bands,
	latestAMI:   true,
}

var distroRHEL8 = distro{
	name:        "rhel8",
	os:          os.RedHat8,
	frameworkID: "cis-rhel8",
	family:      rhel,
	minRules:    200,
	bands:       rhel8Bands,
}

var distroRHEL9 = distro{
	name:        "rhel9",
	os:          os.RedHat9,
	frameworkID: "cis-rhel9",
	family:      rhel,
	minRules:    230,
	bands:       rhel9Bands,
}

// The assertions below are shared by the host and containerized suites. Only the check func differs.

// parseEvents decodes the JSON event stream a compliance check writes to stdout.
func parseEvents(t *testing.T, out string) []event {
	var events []event
	dec := json.NewDecoder(strings.NewReader(jsonOnly(out)))
	for {
		var e event
		err := dec.Decode(&e)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "decoding compliance output")
		events = append(events, e)
	}
	return events
}

// jsonOnly keeps the JSON event lines and drops the log lines the security-agent
// interleaves with them on stdout.
func jsonOnly(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") || strings.HasPrefix(line, "\t") {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// containerErrorCategories lists the rules allowed to error when the agent runs in a
// container: they introspect live systemd unit state (service/socket enablement and
// "is the service active" checks), unreachable through the mounted host root. Any other
// erroring rule is a regression. Entries match the rule suffix as a prefix; the three
// "*_active" rules are named in full so the broad firewall_/logging_/ntp_ prefixes stay
// able to flag config-file regressions. Tuned from CI: only cis-ubuntu2404 errors at the
// pinned policy version (on these 11 rules); the other distros error on none.
var containerErrorCategories = []string{
	"service_",
	"socket_",
	"firewall_single_service_active",
	"logging_services_active",
	"ntp_single_service_active",
}

// assertConsistency checks the host benchmark's result distribution and error rate.
func assertConsistency(t *testing.T, d distro, onHost bool, check func(string) []event) {
	// The host tolerates no errors; the containerized agent may error on rules that read
	// live host state (see containerErrorCategories).
	var allowedErrorCategories []string
	if !onHost {
		allowedErrorCategories = containerErrorCategories
	}

	// A full run also evaluates other benchmarks like cis-docker, so keep only the host one.
	counts := map[string]int{}
	results := map[string]string{}
	total := 0
	for _, e := range check("") {
		if e.FrameworkID != d.frameworkID {
			continue
		}
		total++
		counts[e.Result]++
		results[e.RuleID] = e.Result
	}
	t.Logf("%s: %d rules — passed=%d failed=%d skipped=%d error=%d",
		d.frameworkID, total, counts["passed"], counts["failed"], counts["skipped"], counts["error"])
	require.GreaterOrEqualf(t, total, d.minRules,
		"expected at least %d %s rules, got %d", d.minRules, d.frameworkID, total)

	// Errors mean a probe could not evaluate. On the host none are tolerated. A
	// containerized agent cannot introspect the host's live systemd unit state, so
	// rules in allowedErrorCategories may error there; any other erroring rule is a
	// regression. The set is always logged so green runs surface it too.
	var errored []string
	for rule, res := range results {
		if res == "error" {
			errored = append(errored, rule)
		}
	}
	sort.Strings(errored)
	if len(errored) > 0 {
		t.Logf("%s errored rules: %v", d.frameworkID, errored)
	}
	for _, rule := range errored {
		assert.Truef(t, hasErrorCategory(rule, allowedErrorCategories),
			"%s errored but is not in an allowed-to-error category on %s", rule, d.frameworkID)
	}

	for result, b := range d.bands {
		frac := float64(counts[result]) / float64(total)
		assert.Truef(t, frac >= b.min && frac <= b.max,
			"%s fraction %.2f outside [%.2f, %.2f] (%d/%d)",
			result, frac, b.min, b.max, counts[result], total)
	}

	// The probed rules must actually be evaluated in the full run; a skip or error
	// signals an applicability regression (e.g. a platform check failing on this
	// OS, as with the AL2023 kernel-core issue).
	for _, p := range probesFor(d, onHost) {
		assert.Containsf(t, []string{"passed", "failed"}, results[p.rule],
			"%s must be evaluated on %s but was %q", p.rule, d.frameworkID, results[p.rule])
	}
}

// runProbe checks a rule fails while the host is broken and passes once it is fixed.
func runProbe(t *testing.T, d distro, p probe, run func(string) string, check func(string) []event) {
	run(p.broken)
	assert.Equalf(t, "failed", checkRule(t, d, check, p.rule).Result,
		"%s should fail while the host is non-compliant", p.rule)
	run(p.fixed)
	assert.Equalf(t, "passed", checkRule(t, d, check, p.rule).Result,
		"%s should pass once the host is compliant", p.rule)
}

// checkRule runs a single rule and returns its finding, asserting the finding shape.
// A run can load several bundled benchmarks that share SSG rule IDs, so keep only the
// events for this distro's framework before requiring exactly one.
func checkRule(t *testing.T, d distro, check func(string) []event, rule string) event {
	var events []event
	for _, e := range check(rule) {
		if e.FrameworkID == d.frameworkID {
			events = append(events, e)
		}
	}
	require.Lenf(t, events, 1, "rule %s should yield exactly one %s finding", rule, d.frameworkID)
	e := events[0]
	assert.Equalf(t, rule, e.RuleID, "finding reports the wrong rule")
	assert.Equalf(t, "host", e.ResourceType, "host benchmark finding should target the host")
	// The evaluator sets resource_id to the scanned host's name, so an empty one
	// means the finding cannot be attributed to a resource.
	assert.NotEmptyf(t, e.ResourceID, "finding should identify the host it evaluated")
	return e
}

// hasErrorCategory reports whether rule belongs to one of the allowed-to-error
// categories, matched on the rule suffix (e.g. "service_").
func hasErrorCategory(rule string, categories []string) bool {
	suffix := strings.TrimPrefix(rule, rulePrefix)
	for _, c := range categories {
		if strings.HasPrefix(suffix, c) {
			return true
		}
	}
	return false
}

// resultsByRule maps each host-benchmark rule to its result for one full run.
func resultsByRule(d distro, check func(string) []event) map[string]string {
	m := map[string]string{}
	for _, e := range check("") {
		if e.FrameworkID == d.frameworkID {
			m[e.RuleID] = e.Result
		}
	}
	return m
}

// assertBundledContent checks the agent ships the distro's benchmark and SCAP
// datastream, so a packaging regression fails clearly instead of as "0 rules".
func assertBundledContent(t *testing.T, d distro, listing string) {
	suffix := strings.TrimPrefix(d.frameworkID, "cis-")
	assert.Containsf(t, listing, "cis-"+suffix, "benchmark for %s missing from compliance.d", d.frameworkID)
	assert.Containsf(t, listing, "ssg-"+suffix+"-ds.xml", "SCAP datastream for %s missing from compliance.d", d.frameworkID)
}

// hostBenchmarksSuite runs the benchmarks with the agent installed on the host.
type hostBenchmarksSuite struct {
	e2e.BaseSuite[environments.Host]
	distro distro
}

func testHostBenchmarks(t *testing.T, d distro) {
	t.Parallel()
	instanceOpts := []ec2.VMOption{ec2.WithOS(d.os)}
	if d.latestAMI {
		instanceOpts = append(instanceOpts, ec2.WithLatestAMI())
	}
	e2e.Run(t, &hostBenchmarksSuite{distro: d},
		e2e.WithStackName("cspm-host-"+d.name),
		e2e.WithProvisioner(awshost.Provisioner(awshost.WithRunOptions(
			ec2.WithEC2InstanceOptions(instanceOpts...),
			ec2.WithAgentOptions(agentparams.WithSecurityAgentConfig(hostBenchmarksConfig)),
		))),
	)
}

func TestHostBenchmarksRHEL8(t *testing.T)           { testHostBenchmarks(t, distroRHEL8) }
func TestHostBenchmarksRHEL9(t *testing.T)           { testHostBenchmarks(t, distroRHEL9) }
func TestHostBenchmarksRHEL10(t *testing.T)          { testHostBenchmarks(t, distroRHEL10) }
func TestHostBenchmarksUbuntu2404(t *testing.T)      { testHostBenchmarks(t, distroUbuntu2404) }
func TestHostBenchmarksAmazonLinux2023(t *testing.T) { testHostBenchmarks(t, distroAmazonLinux2023) }
func TestHostBenchmarksAlmaLinux9(t *testing.T)      { testHostBenchmarks(t, distroAlmaLinux9) }

func (s *hostBenchmarksSuite) runHost(cmd string) string {
	return s.Env().RemoteHost.MustExecute(cmd)
}

func (s *hostBenchmarksSuite) check(args string) []event {
	return parseEvents(s.T(), s.runHost(
		fmt.Sprintf("sudo %s compliance check %s 2>/dev/null", securityAgent, args)))
}

// TestBundledContent checks the agent package ships this distro's benchmark content.
func (s *hostBenchmarksSuite) TestBundledContent() {
	assertBundledContent(s.T(), s.distro, s.runHost("sudo ls /etc/datadog-agent/compliance.d/"))
}

func (s *hostBenchmarksSuite) TestConsistency() { assertConsistency(s.T(), s.distro, true, s.check) }

// TestDeterminism re-runs the whole benchmark and asserts identical results,
// catching non-deterministic (flaky) rules.
func (s *hostBenchmarksSuite) TestDeterminism() {
	first := resultsByRule(s.distro, s.check)
	assert.Equal(s.T(), first, resultsByRule(s.distro, s.check),
		"benchmark results differ between identical runs")
}

func (s *hostBenchmarksSuite) TestProbes() {
	for _, p := range probesFor(s.distro, true) {
		s.Run(p.name, func() { runProbe(s.T(), s.distro, p, s.runHost, s.check) })
	}
}

// TestReporting checks compliance findings reach the backend. The fakeintake
// provisioner redirects compliance_config.endpoints, so --report findings land
// at /api/v2/compliance.
func (s *hostBenchmarksSuite) TestReporting() {
	s.runHost(fmt.Sprintf("sudo %s compliance check --report 2>/dev/null", securityAgent))
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		findings, err := s.Env().FakeIntake.Client().GetComplianceFindings()
		require.NoError(c, err)
		reported := false
		for _, f := range findings {
			if f.FrameworkID == s.distro.frameworkID {
				reported = true
				break
			}
		}
		assert.Truef(c, reported, "no %s findings reached fakeintake (%d total)", s.distro.frameworkID, len(findings))
	}, 5*time.Minute, 15*time.Second)
}
