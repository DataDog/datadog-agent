// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && functionaltests && trivy

// Package tests holds tests related files
package tests

import (
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"

	sbompkg "github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom"
	"github.com/DataDog/datadog-agent/pkg/security/rules/bundled"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var (
	_ = declare(TestSBOMHost, sbomTestOpts)
	_ = declare(TestSBOMHostRefresh, sbomTestOpts)
)

// TestSBOMHost checks the package fields of host files against the package
// manager of the host. touch, run on the host, resolves to the package that
// owns it there, and the file it creates in the test root to an empty package
// name.
func TestSBOMHost(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("the processes of the test run in a container")
	}

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_sbom_host_open",
		Expression: `open.file.path == "{{.Root}}/sbom-host" && open.flags & O_CREAT != 0`,
	}}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	file, _, err := test.Path("sbom-host")
	if err != nil {
		t.Fatal(err)
	}

	var touch packageFields
	test.WaitSignalFromRule(t, func() error {
		return exec.Command("touch", file).Run()
	}, func(event *model.Event, _ *rules.Rule) {
		assertFieldEqual(t, event, "process.container.id", "", "touch runs on the host")
		assertFieldEqual(t, event, "open.file.package.name", "", "the file of the test")
		touch = filePackage(t, event, "process.file")
		test.validateOpenSchema(t, event)
	}, "test_sbom_host_open")

	owners, err := hostOwners(touch.path)
	if err != nil {
		t.Fatal(err)
	}
	assert.Containsf(t, owners, touch.owner(), "%s resolves to %s, and the host lists it under %v", touch.path, touch.owner(), owners)
}

// sbomTestPackage is the dpkg package TestSBOMHostRefresh installs, at version
// sbomTestVersion.
const (
	sbomTestPackage = "sbom-host-test"
	sbomTestVersion = "1:1.0-1"
)

// TestSBOMHostRefresh checks the bundled refresh rules against the package
// manager of the host: a query leaves the host index as it is, a write scans
// the host packages again, and on a dpkg host a package installed resolves
// once that scan is over.
func TestSBOMHostRefresh(t *testing.T) {
	SkipIfNotAvailable(t)

	if testEnvironment == DockerEnvironment {
		t.Skip("the processes of the test run in a container")
	}

	ruleDefs := []*rules.RuleDefinition{{
		ID:         "test_sbom_host_exec",
		Expression: `exec.file.name == "` + sbomTestPackage + `"`,
	}}
	test, err := newTestModule(t, nil, ruleDefs)
	if err != nil {
		t.Fatal(err)
	}
	defer test.CloseTest()

	p, ok := test.probe.PlatformProbe.(*sprobe.EBPFProbe)
	if !ok {
		t.Skip("the SBOM resolver runs with the eBPF probe")
	}

	// Some database backends open files read-write for a query, and the
	// refresh rule excludes those files.
	t.Run("query", func(t *testing.T) {
		touch, err := whichNonFatal("touch")
		if err != nil {
			t.Fatal(err)
		}
		list := []string{"rpm", "-qa"}
		if hostDpkg() {
			list = []string{"dpkg", "-l"}
		}

		err = test.getSignalFromRule(t, func() error {
			if _, err := hostOwners(touch); err != nil {
				return err
			}
			if out, err := exec.Command(list[0], list[1:]...).CombinedOutput(); err != nil {
				return fmt.Errorf("%s: %w: %s", strings.Join(list, " "), err, out)
			}
			return nil
		}, func(event *model.Event, _ *rules.Rule) error {
			if event.ProcessContext.PPid != testSuitePid {
				return errSkipEvent
			}
			path, _ := event.GetFieldValue("open.file.path")
			flags, _ := event.GetFieldValue("open.flags")
			return fmt.Errorf("%s opened %v with flags %#o, which refreshes the host SBOM", event.ProcessContext.Comm, path, flags)
		}, bundled.NeedRefreshSBOMRuleID)
		if _, ok := err.(ErrTimeout); !ok {
			t.Fatalf("waiting for the refresh rule: %v", err)
		}
	})

	// A child writes to the database, and the rules refresh the SBOM when it
	// exits.
	t.Run("write", func(t *testing.T) {
		db := "/var/lib/rpm"
		if hostDpkg() {
			db = "/var/lib/dpkg"
		}
		probe := filepath.Join(db, ".sbom-host-refresh-test")
		if err := os.WriteFile(probe, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(probe)

		scans := sbomGenerations(test)
		err := test.getSignalFromRule(t, func() error {
			return exec.Command("sh", "-c", "echo sbom >>"+probe).Run()
		}, func(event *model.Event, _ *rules.Rule) error {
			if event.ProcessContext.PPid != testSuitePid {
				return errSkipEvent
			}
			return nil
		}, bundled.RefreshSBOMRuleID)
		if err != nil {
			t.Fatalf("waiting for the refresh of the write to %s: %v", probe, err)
		}

		err = retry(t, func() error {
			if sbomGenerations(test) == scans {
				return errors.New("no scan of the host packages since the write")
			}
			return nil
		}, backoff.WithBackOff(backoff.NewConstantBackOff(time.Second)), backoff.WithMaxTries(30))
		assert.NoError(t, err)
	})

	t.Run("install", func(t *testing.T) {
		if !hostDpkg() {
			t.Skip("needs a dpkg host")
		}

		deb := buildTestDeb(t)
		run := func(args ...string) {
			if out, err := dpkg(args...); err != nil {
				if bytes.Contains(out, []byte("lock")) {
					t.Skipf("dpkg stayed locked: %s", out)
				}
				t.Fatalf("dpkg %s: %v: %s", strings.Join(args, " "), err, out)
			}
		}
		run("--purge", sbomTestPackage)
		t.Cleanup(func() {
			if out, err := dpkg("--purge", sbomTestPackage); err != nil {
				t.Errorf("dpkg --purge %s: %v: %s", sbomTestPackage, err, out)
			}
		})

		reports, err := hostReports(p.Resolvers.SBOMResolver)
		if err != nil {
			t.Fatal(err)
		}
		run("--install", deb)

		// The daemon of the package started before the scan the installation
		// fired, so the scan records it.
		err = waitHostComponent(reports, sbomTestPackage, 90*time.Second, func(c *cyclonedx_v1_4.Component) error {
			props := make(map[string]string)
			for _, prop := range c.Properties {
				props[prop.Name] = prop.GetValue()
			}
			switch {
			case c.Version != sbomTestVersion:
				return fmt.Errorf("%s has version %s", c.Name, c.Version)
			case props[sbom.LastAccessProperty] == "0":
				return fmt.Errorf("%s is unused, with its daemon running", c.Name)
			case props[sbom.RunningAsRootProperty] != "true":
				return fmt.Errorf("%s runs as root, and %s is %q", c.Name, sbom.RunningAsRootProperty, props[sbom.RunningAsRootProperty])
			}
			return nil
		})
		assert.NoError(t, err)

		var got packageFields
		test.WaitSignalFromRule(t, func() error {
			return exec.Command("/bin/"+sbomTestPackage, "0").Run()
		}, func(event *model.Event, _ *rules.Rule) {
			got = filePackage(t, event, "exec.file")
		}, "test_sbom_host_exec")
		assert.Equal(t, sbomTestPackage+"@"+sbomTestVersion, got.owner(), "the package of %s", got.path)
	})
}

// sbomGenerations returns the count of the scans the SBOM resolver of test
// started, since the statsd client of test was last flushed.
func sbomGenerations(test *testModule) int64 {
	test.eventMonitor.SendStats()
	return test.statsdClient.Get(metrics.MetricSBOMResolverSBOMGenerations)
}

// packageFields are the path and the package fields of a file of an event.
type packageFields struct {
	path, name, version, release string
	epoch                        int
}

// owner names the package of the file as hostOwners does.
func (f packageFields) owner() string {
	return f.name + "@" + packageVersion(f.version, f.release, f.epoch)
}

// filePackage copies out of event the fields of the file prefix names, such
// as process.file, as the event goes back to its pool after the callback.
func filePackage(tb testing.TB, event *model.Event, prefix string) packageFields {
	tb.Helper()
	return packageFields{
		path:    eventField[string](tb, event, prefix+".path"),
		name:    eventField[string](tb, event, prefix+".package.name"),
		version: eventField[string](tb, event, prefix+".package.version"),
		release: eventField[string](tb, event, prefix+".package.release"),
		epoch:   eventField[int](tb, event, prefix+".package.epoch"),
	}
}

// eventField returns the value of field in event, and the zero value after
// reporting an error.
func eventField[T any](tb testing.TB, event *model.Event, field string) T {
	tb.Helper()
	value, err := event.GetFieldValue(field)
	if err != nil {
		tb.Errorf("%s: %v", field, err)
	}
	v, _ := value.(T)
	return v
}

// packageVersion formats a version as dpkg and the SBOM resolver do,
// [epoch:]version[-release].
func packageVersion(version, release string, epoch int) string {
	if release != "" {
		version += "-" + release
	}
	if epoch > 0 {
		version = strconv.Itoa(epoch) + ":" + version
	}
	return version
}

// hostDpkg reports whether dpkg holds the packages of the host, which rpm
// holds otherwise.
func hostDpkg() bool {
	_, err := os.Stat("/var/lib/dpkg/status")
	return err == nil
}

// hostOwners returns the packages the package manager of the host lists path
// under, as name@version.
func hostOwners(path string) (map[string]bool, error) {
	if hostDpkg() {
		return dpkgOwners(path)
	}
	return rpmOwners(path)
}

// dpkgOwners returns the packages dpkg lists path under. dpkg knows a file by
// the path its package installed it at, which on a merged /usr may be the
// alias of the path the kernel reports.
func dpkgOwners(path string) (map[string]bool, error) {
	args := []string{"-S", path}
	if alias := usrMergeAlias(path); alias != "" {
		args = append(args, alias)
	}
	// The exit status of dpkg-query is 1 when it lists one path of two, as it
	// does for a path and its alias.
	cmd := exec.Command("dpkg-query", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, _ := cmd.Output()

	owners := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		names, _, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		for _, name := range strings.Split(names, ", ") {
			// A diversion line, such as "diversion by dash from: /bin/sh",
			// holds words in place of package names.
			if strings.Contains(name, " ") {
				continue
			}
			name, _, _ = strings.Cut(name, ":")
			versions, err := exec.Command("dpkg-query", "-W", "-f", "${Version}\n", name).Output()
			if err != nil {
				return nil, fmt.Errorf("dpkg-query -W %s: %w", name, err)
			}
			for _, version := range strings.Fields(string(versions)) {
				owners[name+"@"+version] = true
			}
		}
	}
	if len(owners) == 0 {
		return nil, fmt.Errorf("dpkg-query -S %s printed no package: %q", path, out)
	}
	return owners, nil
}

// usrMergeAlias returns the name path also has on a merged /usr, where /bin,
// /sbin and /lib link into /usr, or "" for a path outside them.
func usrMergeAlias(path string) string {
	for _, dir := range []string{"/bin/", "/sbin/", "/lib"} {
		if strings.HasPrefix(path, "/usr"+dir) {
			return strings.TrimPrefix(path, "/usr")
		}
		if strings.HasPrefix(path, dir) {
			return "/usr" + path
		}
	}
	return ""
}

// rpmOwners returns the packages rpm lists path under.
func rpmOwners(path string) (map[string]bool, error) {
	out, err := exec.Command("rpm", "-qf", "--queryformat", "%{NAME} %{EPOCH} %{VERSION} %{RELEASE}\n", path).Output()
	if err != nil {
		return nil, fmt.Errorf("rpm -qf %s: %w: %s", path, err, out)
	}

	owners := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			return nil, fmt.Errorf("rpm -qf %s printed %q", path, line)
		}
		// rpm prints (none) for an unset epoch, which reads as 0.
		epoch, _ := strconv.Atoi(fields[1])
		owners[fields[0]+"@"+packageVersion(fields[2], fields[3], epoch)] = true
	}
	return owners, nil
}

// buildTestDeb builds sbomTestPackage, whose script sleeps for the seconds of
// its argument. Its postinst runs the script as a daemon, as the postinst of a
// server package starts its daemon, and its prerm stops it. The script lies
// under /bin, which a merged /usr turns into an alias.
func buildTestDeb(t *testing.T) string {
	t.Helper()

	const script = "#!/bin/sh\nsleep \"$1\"\n"
	pidFile := "/run/" + sbomTestPackage + ".pid"
	files := []struct {
		name    string
		mode    os.FileMode
		content string
	}{
		{"bin/" + sbomTestPackage, 0o755, script},
		{"DEBIAN/control", 0o644, "Package: " + sbomTestPackage + "\n" +
			"Version: " + sbomTestVersion + "\n" +
			"Architecture: all\n" +
			"Maintainer: Datadog <package@datadoghq.com>\n" +
			"Description: package of the SBOM functional tests\n"},
		// The SBOM resolver lists the files of a dpkg package from its md5sums.
		{"DEBIAN/md5sums", 0o644, fmt.Sprintf("%x  bin/%s\n", md5.Sum([]byte(script)), sbomTestPackage)},
		{"DEBIAN/postinst", 0o755, "#!/bin/sh\n" +
			"setsid /bin/" + sbomTestPackage + " 600 </dev/null >/dev/null 2>&1 &\n" +
			"echo $! >" + pidFile + "\n"},
		{"DEBIAN/prerm", 0o755, "#!/bin/sh\n" +
			"if [ -f " + pidFile + " ]; then\n" +
			"\tpkill -g \"$(cat " + pidFile + ")\"\n" +
			"\trm -f " + pidFile + "\n" +
			"fi\n" +
			"exit 0\n"},
	}

	// dpkg-deb checks the modes of the control directory and of the scripts,
	// which the umask narrows.
	root := filepath.Join(t.TempDir(), sbomTestPackage)
	for _, dir := range []string{root, filepath.Join(root, "bin"), filepath.Join(root, "DEBIAN")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range files {
		path := filepath.Join(root, f.name)
		if err := os.WriteFile(path, []byte(f.content), f.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, f.mode); err != nil {
			t.Fatal(err)
		}
	}

	deb := root + ".deb"
	if out, err := exec.Command("dpkg-deb", "--build", root, deb).CombinedOutput(); err != nil {
		t.Fatalf("dpkg-deb --build: %v: %s", err, out)
	}
	return deb
}

// dpkg runs dpkg with args, again while another process holds its lock, as the
// package timers of a distribution may do for minutes, and returns the output
// of its last run.
func dpkg(args ...string) ([]byte, error) {
	deadline := time.Now().Add(3 * time.Minute)
	for {
		cmd := exec.Command("dpkg", args...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		out, err := cmd.CombinedOutput()
		if err == nil || !bytes.Contains(out, []byte("lock")) || time.Now().After(deadline) {
			return out, err
		}
		time.Sleep(5 * time.Second)
	}
}

// hostReports returns the host SBOM reports resolver forwards from now on. A
// resolver keeps its listeners and calls them with the SBOM locked, so the
// listener drops a report while the channel is full.
func hostReports(resolver *sbom.Resolver) (<-chan sbompkg.Report, error) {
	reports := make(chan sbompkg.Report, 8)
	err := resolver.RegisterListener(sbom.SBOMComputed, func(result *sbompkg.ScanResult) {
		if result.RequestID != "" {
			return
		}
		select {
		case reports <- result.Report:
		default:
		}
	})
	return reports, err
}

// waitHostComponent waits for a host report in which check accepts the
// component name, and returns the last error of check on timeout.
func waitHostComponent(reports <-chan sbompkg.Report, name string, timeout time.Duration, check func(*cyclonedx_v1_4.Component) error) error {
	err := fmt.Errorf("no host report in %s", timeout)
	deadline := time.After(timeout)
	for {
		select {
		case report := <-reports:
			err = fmt.Errorf("the host report lists no %s", name)
			for _, c := range report.ToCycloneDX().Components {
				if c.Name != name {
					continue
				}
				if err = check(c); err == nil {
					return nil
				}
			}
		case <-deadline:
			return err
		}
	}
}
