// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package main is the test-runner tool which runs the system-probe tests
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/sys/unix"
)

const matchAllPackages = "*"

func init() {
	color.NoColor = false
}

type packageRunConfiguration struct {
	RunOnly []string `json:"run-only"`
	Skip    []string
	Exclude bool
}

type testConfig struct {
	userProvidedConfig
	runCount     int
	retryCount   int
	verbose      bool
	testDirRoot  string
	testingTools string
	extraParams  string
	extraEnv     string
}

type userProvidedConfig struct {
	PackagesRunConfig  map[string]packageRunConfiguration `json:"filters"`
	InContainerImage   string                             `json:"testcontainer"`
	AdditionalTestArgs []string                           `json:"additional_test_args"`
	AdditionalEnvVars  []string                           `json:"additional_env_vars"`
}

const ciVisibility = "/ci-visibility"

var baseEnv = []string{
	"GITLAB_CI=true", // force color output support to be detected
	"GOVERSION=" + runtime.Version(),
}

// change `TEST_TIMEOUTS` in `tasks/system_probe.py` if you change them here
var timeouts = map[*regexp.Regexp]time.Duration{
	regexp.MustCompile("pkg/network/protocols$"):      5 * time.Minute,
	regexp.MustCompile("pkg/network/protocols/http$"): 15 * time.Minute,
	regexp.MustCompile("pkg/network/tracer$"):         55 * time.Minute,
	regexp.MustCompile("pkg/network/usm$"):            55 * time.Minute,
	regexp.MustCompile("pkg/network/usm/tests$"):      20 * time.Minute,
	regexp.MustCompile("pkg/security.*"):              45 * time.Minute,
}

func getTimeout(pkg string) time.Duration {
	matchSize := 0
	to := 10 * time.Minute
	for re, rto := range timeouts {
		if re.MatchString(pkg) && len(re.String()) > matchSize {
			matchSize = len(re.String())
			to = rto
		}
	}
	return to
}

func getEBPFBuildDir() (string, error) {
	arch, _, err := getArchAndRelease()
	if err != nil {
		return "", fmt.Errorf("cannot get arch: %w", err)
	}
	if arch == "aarch64" {
		arch = "arm64"
	}

	return fmt.Sprintf("pkg/ebpf/bytecode/build/%s", arch), nil
}

func glob(dir, filePattern string, filterFn func(path string) bool) ([]string, error) {
	var matches []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		present, err := regexp.Match(filePattern, []byte(d.Name()))
		if err != nil {
			return fmt.Errorf("file regexp match: %s", err)
		}

		if d.IsDir() || !present {
			return nil
		}
		if filterFn(path) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return matches, nil
}

func buildCommandArgs(pkg string, xmlpath string, jsonpath string, testArgs []string, testConfig *testConfig) []string {
	verbosity := "testname"
	if testConfig.verbose {
		verbosity = "standard-verbose"
	}

	args := []string{
		"--format", verbosity,
		"--junitfile", xmlpath,
		"--jsonfile", jsonpath,
		fmt.Sprintf("--rerun-fails=%d", testConfig.retryCount),
		"--rerun-fails-max-failures=100",
		"--raw-command", "--",
		filepath.Join(testConfig.testingTools, "go/bin/test2json"), "-t", "-p", pkg,
	}
	args = append(args, testArgs...)
	args = append(
		args,
		"-test.v",
		fmt.Sprintf("-test.count=%d", testConfig.runCount),
		"-test.timeout="+getTimeout(pkg).String(),
	)

	if testConfig.extraParams != "" {
		args = append(args, strings.Split(testConfig.extraParams, " ")...)
	}
	args = append(args, testConfig.AdditionalTestArgs...)

	packagesRunConfig := testConfig.PackagesRunConfig
	if config, ok := packagesRunConfig[pkg]; ok && config.RunOnly != nil {
		args = append(args, "-test.run", strings.Join(config.RunOnly, "|"))
	}
	if config, ok := packagesRunConfig[pkg]; ok && config.Skip != nil {
		args = append(args, "-test.skip", strings.Join(config.Skip, "|"))
	}

	return args
}

// concatenateJsons combines all the test json output files into a single file.
func concatenateJsons(indir, outdir string) error {
	testJSONFile := filepath.Join(outdir, "out.json")
	matches, err := glob(indir, `.*\.json`, func(_ string) bool { return true })
	if err != nil {
		return fmt.Errorf("json glob: %s", err)
	}

	f, err := os.OpenFile(testJSONFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return fmt.Errorf("open %s: %s", testJSONFile, err)
	}
	defer f.Close()

	for _, jsonFile := range matches {
		jf, err := os.Open(jsonFile)
		if err != nil {
			return fmt.Errorf("open %s: %s", jsonFile, err)
		}

		_, err = io.Copy(f, jf)
		_ = jf.Close()
		if err != nil {
			return fmt.Errorf("%s copy: %s", jsonFile, err)
		}
	}
	return nil
}

func createDir(d string) error {
	if err := os.MkdirAll(d, 0o777); err != nil {
		return fmt.Errorf("failed to create directory %s", d)
	}
	return nil
}

func collectEnvVars(testConfig *testConfig, bpfDir string) []string {
	var env []string
	env = append(env, baseEnv...)
	env = append(env,
		"DD_SYSTEM_PROBE_BPF_DIR="+bpfDir,
		"DD_SERVICE_MONITORING_CONFIG_TLS_JAVA_DIR="+filepath.Join(testConfig.testDirRoot, "pkg/network/protocols/tls/java"),
	)

	if testConfig.extraEnv != "" {
		env = append(env, strings.Split(testConfig.extraEnv, " ")...)
	}

	env = append(env, testConfig.AdditionalEnvVars...)

	return env
}

func testPass(testConfig *testConfig, props map[string]string) error {
	testsuites, err := glob(testConfig.testDirRoot, "testsuite", func(path string) bool {
		dir, _ := filepath.Rel(testConfig.testDirRoot, filepath.Dir(path))

		if config, ok := testConfig.PackagesRunConfig[dir]; ok {
			return !config.Exclude
		}

		if config, ok := testConfig.PackagesRunConfig[matchAllPackages]; ok {
			return !config.Exclude
		}

		return false
	})
	if err != nil {
		return fmt.Errorf("test glob: %s", err)
	}

	jsonDir := filepath.Join(ciVisibility, "pkgjson")
	jsonOutDir := filepath.Join(ciVisibility, "testjson")
	xmlDir := filepath.Join(ciVisibility, "junit")
	for _, d := range []string{jsonDir, jsonOutDir, xmlDir} {
		if err := createDir(d); err != nil {
			return err
		}
	}

	buildDir, err := getEBPFBuildDir()
	if err != nil {
		return fmt.Errorf("getEBPFBuildDir: %w", err)
	}
	bpfDir := filepath.Join(testConfig.testDirRoot, buildDir)

	envVars := collectEnvVars(testConfig, bpfDir)

	var testContainer *testContainer
	if testConfig.InContainerImage != "" {
		testContainer = newTestContainer(testConfig.InContainerImage, bpfDir)
		if err := testContainer.start(); err != nil {
			return fmt.Errorf("error creating test container: %w", err)
		}
	}

	for _, testsuite := range testsuites {
		pkg, err := filepath.Rel(testConfig.testDirRoot, filepath.Dir(testsuite))
		if err != nil {
			return fmt.Errorf("could not get relative path for %s: %w", testsuite, err)
		}
		junitfilePrefix := strings.ReplaceAll(pkg, "/", "-")
		xmlpath := filepath.Join(xmlDir, fmt.Sprintf("%s.xml", junitfilePrefix))
		jsonpath := filepath.Join(jsonDir, fmt.Sprintf("%s.json", junitfilePrefix))

		testsuiteArgs := []string{testsuite}
		if testContainer != nil {
			testsuiteArgs = testContainer.buildDockerExecArgs(testsuite, envVars)
		}

		args := buildCommandArgs(pkg, xmlpath, jsonpath, testsuiteArgs, testConfig)
		cmd := exec.Command(filepath.Join(testConfig.testingTools, "go/bin/gotestsum"), args...)

		cmd.Env = append(cmd.Environ(), envVars...)

		cmd.Dir = filepath.Dir(testsuite)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// log but do not return error
			fmt.Fprintf(os.Stderr, "cmd run %s: %s\n", testsuite, err)
		}

		if err := addProperties(xmlpath, props); err != nil {
			return fmt.Errorf("xml add props: %s", err)
		}
	}

	if err := concatenateJsons(jsonDir, jsonOutDir); err != nil {
		return fmt.Errorf("concat json: %s", err)
	}
	return nil
}

func getRealPath(dir string) (string, error) {
	target := dir

	if fi, err := os.Lstat(target); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, err = os.Readlink(target)
		if err != nil {
			return "", fmt.Errorf("unable to resolve symlink for %s: %w", target, err)
		}
	}

	return target, nil
}

func buildTestConfiguration() (*testConfig, error) {
	retryPtr := flag.Int("retry", 2, "number of times to retry testing pass")
	packageRunConfigPtr := flag.String("packages-run-config", "", "Configuration for controlling which tests run in a package")
	verbose := flag.Bool("verbose", false, "if set to true verbosity level is 'standard-verbose', otherwise it is 'testname'")
	runCount := flag.Int("run-count", 1, "number of times to run the test")
	testRoot := flag.String("test-root", "/opt/system-probe-tests", "directory containing test packages")
	testTools := flag.String("test-tools", "/opt/testing-tools", "directory containing test tools")
	extraParams := flag.String("extra-params", "", "extra parameters to pass to the test runner")
	extraEnv := flag.String("extra-env", "", "extra environment variables to pass to the test runner")

	flag.Parse()

	var userConfig userProvidedConfig
	if *packageRunConfigPtr != "" {
		configData, err := os.ReadFile(*packageRunConfigPtr)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		fmt.Printf("Runner under configuration: %s\n", configData)

		dec := json.NewDecoder(bytes.NewReader(configData))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&userConfig); err != nil {
			return nil, err
		}
	}

	// get real path because the `WalkDir` helper does not support following symlinks
	root, err := getRealPath(*testRoot)
	if err != nil {
		return nil, err
	}

	tools, err := getRealPath(*testTools)
	if err != nil {
		return nil, err
	}

	return &testConfig{
		userProvidedConfig: userConfig,
		runCount:           *runCount,
		verbose:            *verbose,
		retryCount:         *retryPtr,
		testDirRoot:        root,
		testingTools:       tools,
		extraParams:        *extraParams,
		extraEnv:           *extraEnv,
	}, nil
}

func readOSRelease() (map[string]string, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	keyvals := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// output contents for visibility
		fmt.Println(scanner.Text())
		k, v, found := bytes.Cut(scanner.Bytes(), []byte{'='})
		if found {
			keyvals[string(k)] = strings.Trim(string(v), "\"")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return keyvals, nil
}

func getArchAndRelease() (string, string, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "", "", fmt.Errorf("uname: %w", err)
	}
	arch, release := unix.ByteSliceToString(u.Machine[:]), unix.ByteSliceToString(u.Release[:])
	return arch, release, nil
}

func getProps() (map[string]string, error) {
	osrHash, err := readOSRelease()
	if err != nil {
		return nil, fmt.Errorf("os-release: %s", err)
	}
	osname := fmt.Sprintf("%s-%s", osrHash["ID"], osrHash["VERSION_ID"])
	arch, release, err := getArchAndRelease()
	if err != nil {
		return nil, fmt.Errorf("arch and release: %s", err)
	}
	fmt.Printf("arch: %s\nrelease: %s\n", arch, release)
	return map[string]string{
		"dd_tags[os.platform]":     "linux",
		"dd_tags[os.name]":         osname,
		"dd_tags[os.architecture]": arch,
		"dd_tags[os.version]":      release,
	}, nil
}

func pathEmbedded(fullPath, embedded string) bool {
	normalized := fmt.Sprintf("/%s/", strings.Trim(embedded, "/"))

	return strings.Contains(fullPath, normalized)
}

func fixAssetPermissions(testDirRoot string) error {
	matches, err := glob(testDirRoot, `.*\.o`, func(path string) bool {
		return pathEmbedded(path, "pkg/ebpf/bytecode/build")
	})
	if err != nil {
		return fmt.Errorf("glob assets: %w", err)
	}

	for _, file := range matches {
		if err := os.Chown(file, 0, 0); err != nil {
			return fmt.Errorf("chown %s: %w", file, err)
		}
	}
	return nil
}

func run() error {
	props, err := getProps()
	if err != nil {
		return fmt.Errorf("props: %s", err)
	}

	testConfig, err := buildTestConfiguration()
	if err != nil {
		return fmt.Errorf("failed to build test configuration: %w", err)
	}

	if err := fixAssetPermissions(testConfig.testDirRoot); err != nil {
		return fmt.Errorf("asset perms: %s", err)
	}

	if err := os.RemoveAll(ciVisibility); err != nil {
		return fmt.Errorf("failed to remove contents of %s: %w", ciVisibility, err)
	}
	if err := createDir(ciVisibility); err != nil {
		return err
	}

	return testPass(testConfig, props)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", color.RedString(err.Error()))
		os.Exit(1)
	}
}
