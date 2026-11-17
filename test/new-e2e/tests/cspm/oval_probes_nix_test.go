// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	oscapBin   = "/opt/datadog-agent/embedded/bin/oscap"
	ovalDir    = "/var/tmp/dd-oval-probes"
	ovalPrefix = "oval:com.datadoghq.e2e:def:"
)

// ovalProbes holds one definition per OVAL probe the agent's OpenSCAP build
// enables, numbered from 1 in the order of ovalExpected.
//
//go:embed testdata/oval/probes.xml
var ovalProbes []byte

// OVAL definition results, as oscap prints them. oscapFailed marks a run
// that printed no result, as when a probe aborts the evaluation.
const (
	ovalTrue    = "true"
	ovalFalse   = "false"
	ovalError   = "error"
	ovalUnknown = "unknown"
	ovalNA      = "not applicable"
	oscapFailed = "oscap failed"
)

// ovalProbe is one definition of testdata/oval/probes.xml and the result it
// gives on each family, on the host and through OSCAP_PROBE_ROOT, the way
// the containerized agent scans the host.
type ovalProbe struct {
	name      string
	host      [2]string // indexed by family
	container [2]string
}

func both(r string) [2]string { return [2]string{r, r} }

// ovalExpected lists the definitions in file order. The libraries a probe
// exercises are in the definition's description. file_acl takes its result
// from whether the image has setfacl (see ovalFixtures).
var ovalExpected = []ovalProbe{
	{"family", both(ovalTrue), both(ovalTrue)},
	{"uname", both(ovalTrue), both(ovalNA)},
	{"environmentvariable", both(ovalTrue), both(ovalNA)},
	{"environmentvariable58", both(ovalTrue), both(ovalTrue)},
	{"variable", both(ovalTrue), both(ovalTrue)},
	{"textfilecontent", both(ovalTrue), both(ovalTrue)},
	{"textfilecontent54", both(ovalTrue), both(ovalTrue)},
	{"xmlfilecontent", both(ovalTrue), both(ovalTrue)},
	{"filehash58", both(ovalTrue), both(ovalTrue)},
	{"file", both(ovalTrue), both(ovalTrue)},
	{"file_acl", both(""), both("")},
	{"fileextendedattribute", both(ovalTrue), both(ovalTrue)},
	{"symlink", both(ovalTrue), both(ovalTrue)},
	{"password", both(ovalTrue), both(ovalTrue)},
	{"shadow", both(ovalTrue), both(ovalTrue)},
	{"process", both(ovalTrue), both(ovalNA)},
	{"interface", both(ovalTrue), both(ovalNA)},
	{"routingtable", both(ovalTrue), both(ovalNA)},
	{"inetlisteningservers", both(ovalTrue), both(ovalTrue)},
	{"iflisteners", both(ovalTrue), both(ovalTrue)},
	{"runlevel", both(ovalTrue), both(ovalTrue)},
	{"xinetd", both(ovalTrue), both(ovalTrue)},
	{"sysctl", both(ovalTrue), both(ovalNA)},
	{"dnscache", both(ovalTrue), both(ovalTrue)},
	{"dpkginfo", [2]string{ovalError, ovalTrue}, [2]string{ovalError, ovalTrue}},
	{"rpminfo", [2]string{ovalTrue, ovalFalse}, [2]string{ovalTrue, ovalFalse}},
	{"rpmverify", [2]string{ovalTrue, ovalFalse}, [2]string{ovalTrue, ovalFalse}},
	{"rpmverifyfile", [2]string{ovalTrue, ovalFalse}, [2]string{ovalTrue, ovalFalse}},
	{"rpmverifypackage", [2]string{ovalTrue, ovalFalse}, both(ovalNA)},
	{"selinuxboolean", [2]string{ovalTrue, oscapFailed}, [2]string{oscapFailed, ovalFalse}},
	{"selinuxsecuritycontext", [2]string{ovalTrue, ovalError}, [2]string{ovalTrue, ovalError}},
	{"partition", both(ovalTrue), both(ovalFalse)},
	{"systemdunitproperty", both(ovalTrue), [2]string{ovalTrue, ovalUnknown}},
	{"systemdunitdependency", both(ovalTrue), [2]string{ovalTrue, ovalUnknown}},
	{"fwupdsecattr", both(ovalError), both(ovalNA)},
}

// ovalFixtures creates the files the definitions read and prints "acl" when
// setfacl could give f.acl an ACL. The user.dd attribute is set with the
// agent's Python on the host, or the system's in the containerized suite.
const ovalFixtures = `set -e
sudo rm -rf ` + ovalDir + `
mkdir -p ` + ovalDir + ` && cd ` + ovalDir + `
printf 'dd-oval-probe\nkey = value\n' > f.txt
printf '<root><item name="dd">probe</item></root>\n' > f.xml
ln -s f.txt link
: > f.acl
py=/opt/datadog-agent/embedded/bin/python3
[ -x "$py" ] || py=$(command -v python3 || echo /usr/libexec/platform-python)
"$py" -c 'import os; os.setxattr("f.txt", "user.dd", b"probe")'
if command -v setfacl >/dev/null && setfacl -m u:nobody:r f.acl; then echo acl; fi
`

// ovalEvaluate returns the script that evaluates each definition of file in
// its own oscap run, so that a probe which aborts the evaluation spoils only
// its own result, and prints "<n>\t<result>\t<first message>" per definition.
// The result comes from oscap's output, since a probe running chrooted can
// keep oscap from writing its results file. invoke runs oscap.
func ovalEvaluate(invoke, file, resultsDir string) string {
	return fmt.Sprintf(`for n in $(seq 1 %d); do
out=$(%s oval eval --skip-valid --id %s$n --results %s/r$n.xml %s 2>&1)
res=$(printf '%%s\n' "$out" | sed -n 's/^Definition %s[0-9]*: //p' | head -n 1)
msg=$(grep -o '<message[^>]*>[^<]*' %s/r$n.xml 2>/dev/null | head -n 1 | sed 's/^<message[^>]*>//')
[ -n "$res" ] || msg=$(printf '%%s\n' "$out" | grep -v '^W:' | head -n 1)
printf '%%s\t%%s\t%%s\n' "$n" "${res:-%s}" "$msg"
done`, len(ovalExpected), invoke, ovalPrefix, resultsDir, file, ovalPrefix, resultsDir, oscapFailed)
}

// ovalResult is the outcome of one definition.
type ovalResult struct{ result, message string }

// parseOVALResults decodes the lines ovalEvaluate prints.
func parseOVALResults(t *testing.T, out string) map[int]ovalResult {
	m := map[int]ovalResult{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) < 2 {
			continue
		}
		n, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		r := ovalResult{result: f[1]}
		if len(f) == 3 {
			r.message = f[2]
		}
		m[n] = r
	}
	require.Lenf(t, m, len(ovalExpected), "oscap printed results for %d of %d definitions:\n%s", len(m), len(ovalExpected), out)
	return m
}

// assertOVALProbes checks every definition against ovalExpected, logs the
// whole table and saves it as an artifact named after the framework and env.
func assertOVALProbes(t *testing.T, d distro, onHost bool, acl bool, outDir string, results map[int]ovalResult) {
	env := "host"
	if !onHost {
		env = "container"
	}
	var table []string
	for i, p := range ovalExpected {
		n := i + 1
		got := results[n]
		want := p.host[d.family]
		if !onHost {
			want = p.container[d.family]
		}
		if p.name == "file_acl" {
			want = ovalFalse
			if acl {
				want = ovalTrue
			}
		}
		table = append(table, fmt.Sprintf("%2d %-24s %-15s %s", n, p.name, got.result, got.message))
		assert.Equalf(t, want, got.result, "OVAL probe %s on %s (%s): message %q", p.name, d.name, env, got.message)
		// fwupd is absent from the images, so the probe reports an error, but
		// that error must still come from fwupd: a libdbus failure reads
		// "D-Bus connection failed".
		if p.name == "fwupdsecattr" && onHost {
			assert.NotContainsf(t, got.message, "D-Bus connection failed", "fwupdsecattr on %s could not reach D-Bus", d.name)
		}
	}
	report := strings.Join(table, "\n")
	t.Logf("%s (%s) OVAL probes:\n%s", d.frameworkID, env, report)
	writeArtifact(t, outDir, "oval-probes-"+d.frameworkID+"-"+env+".txt", report)
}

// TestOVALProbes evaluates one OVAL definition per enabled probe with the
// bundled oscap on the host.
func (s *hostBenchmarksSuite) TestOVALProbes() {
	acl := strings.Contains(s.runHost(ovalFixtures), "acl")
	s.T().Cleanup(func() { s.runHost("sudo rm -rf " + ovalDir) })
	_, err := s.Env().RemoteHost.WriteFile(ovalDir+"/probes.xml", ovalProbes)
	require.NoError(s.T(), err)
	out := s.runHost(ovalEvaluate("sudo "+oscapBin, ovalDir+"/probes.xml", ovalDir))
	assertOVALProbes(s.T(), s.distro, true, acl, s.SessionOutputDir(), parseOVALResults(s.T(), out))
}

// TestOVALProbes evaluates the same definitions inside the agent container
// against the host root mounted at /host, with OSCAP_PROBE_ROOT=/host as
// the agent sets it.
func (s *containerBenchmarksSuite) TestOVALProbes() {
	acl := strings.Contains(s.runHost(ovalFixtures), "acl")
	s.T().Cleanup(func() { s.runHost("sudo rm -rf " + ovalDir) })
	_, err := s.Env().RemoteHost.WriteFile(ovalDir+"/probes.xml", ovalProbes)
	require.NoError(s.T(), err)
	script := ovalEvaluate("env OSCAP_PROBE_ROOT=/host "+oscapBin, "/host"+ovalDir+"/probes.xml", "/tmp")
	out := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "sh", "-c", script)
	assertOVALProbes(s.T(), s.distro, false, acl, s.SessionOutputDir(), parseOVALResults(s.T(), out))
}
