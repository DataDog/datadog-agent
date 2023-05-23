// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	Testsuite   = "testsuite"
	TestDirRoot = "/opt/system-probe-tests"
	GoTestSum   = "/go/bin/gotestsum"
)

var BaseEnv = map[string]interface{}{
	"DD_SYSTEM_PROBE_BPF_DIR":  filepath.Join(TestDirRoot, "pkg/ebpf/bytecode/build"),
	"DD_SYSTEM_PROBE_JAVA_DIR": filepath.Join(TestDirRoot, "pkg/network/java"),
}

type testConfig struct {
	bundle         string
	env            map[string]interface{}
	filterPackages filterPaths
}

type filterPaths struct {
	paths     []string
	inclusive bool
}

var skipPrebuiltTests = filterPaths{
	paths:     []string{"pkg/collector/corechecks/ebpf/probe"},
	inclusive: false,
}

var runtimeCompiledTests = filterPaths{
	paths: []string{
		"pkg/network/tracer",
		"pkg/network/protocols/http",
		"pkg/collector/corechecks/ebpf/probe",
	},
	inclusive: true,
}

var coreTests = filterPaths{
	paths: []string{
		"pkg/collector/corechecks/ebpf/probe",
		"pkg/network/protocols/http",
	},
	inclusive: true,
}

var fentryTests = filterPaths{
	paths:     skipPrebuiltTests.paths,
	inclusive: false,
}

var timeouts = map[*regexp.Regexp]time.Duration{
	regexp.MustCompile("pkg/network/protocols/http$"): 15 * time.Minute,
	regexp.MustCompile("pkg/network/tracer$"):         25 * time.Minute,
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

func pathEmbedded(fullPath, embedded string) bool {
	normalized := fmt.Sprintf("/%s/", strings.Trim(embedded, "/"))

	return strings.Contains(fullPath, normalized)
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

func generatePackageName(file string) string {
	pkg, _ := filepath.Rel(TestDirRoot, filepath.Dir(file))
	return pkg
}

func buildCommandArgs(file, bundle string) []string {
	pkg := generatePackageName(file)
	junitfilePrefix := strings.ReplaceAll(pkg, "/", "-")
	xmlpath := filepath.Join(
		"/", "junit", bundle,
		fmt.Sprintf("%s.xml", junitfilePrefix),
	)
	jsonpath := filepath.Join(
		"/", "pkgjson", bundle,
		fmt.Sprintf("%s.json", junitfilePrefix),
	)
	args := []string{
		"--format", "dots",
		"--junitfile", xmlpath,
		"--jsonfile", jsonpath,
		"--raw-command", "--",
		"/go/bin/test2json", "-t", "-p", pkg, file, "-test.v", "-test.count=1", "-test.timeout=" + getTimeout(pkg).String(),
	}

	return args
}

func mergeEnv(env ...map[string]interface{}) []string {
	var mergedEnv []string

	for _, e := range env {
		for key, element := range e {
			mergedEnv = append(mergedEnv, fmt.Sprintf("%s=%s", key, fmt.Sprint(element)))
		}
	}

	return mergedEnv
}

func filterPackagesFn(filter filterPaths) func(path string) bool {
	return func(path string) bool {
		for _, p := range filter.paths {
			if pathEmbedded(path, p) && filter.inclusive {
				return true
			} else if !pathEmbedded(path, p) && !filter.inclusive {
				return true
			}
		}

		return false
	}
}

func concatenateBundleJsons(bundle string) error {
	testJsonFile := filepath.Join("/", "testjson", fmt.Sprintf("%s.json", bundle))
	bundleJSONPath := filepath.Join("/", "pkgjson", bundle)
	matches, err := glob(bundleJSONPath, `*\.json`, func(path string) bool { return true })
	if err != nil {
		return fmt.Errorf("json glob: %s", err)
	}

	f, err := os.OpenFile(testJsonFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("open %s: %s", testJsonFile, err)
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

func testPass(config testConfig) error {
	matches, err := glob(TestDirRoot, Testsuite, filterPackagesFn(config.filterPackages))
	if err != nil {
		return fmt.Errorf("test glob: %s", err)
	}

	if err := os.RemoveAll("/junit/"); err != nil {
		return fmt.Errorf("failed to remove contents of /junit/: %w", err)
	}
	if err := os.RemoveAll("/pkgjson/"); err != nil {
		return fmt.Errorf("failed to remove contents of /pkgjson/: %w", err)
	}

	bundleXMLPath := filepath.Join("/", "junit", config.bundle)
	bundleJSONPath := filepath.Join("/", "pkgjson", config.bundle)

	// create bundle if not exist
	if _, err := os.Stat(bundleXMLPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(bundleXMLPath, 0777); err != nil {
			return fmt.Errorf("failed to create directory %s", bundleXMLPath)
		}
	}
	if _, err := os.Stat(bundleJSONPath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(bundleJSONPath, 0777); err != nil {
			return fmt.Errorf("failed to create directory %s", bundleJSONPath)
		}
	}

	for _, file := range matches {
		args := buildCommandArgs(file, config.bundle)
		cmd := exec.Command(GoTestSum, args...)

		cmd.Env = append(cmd.Environ(), mergeEnv(config.env, BaseEnv)...)
		cmd.Dir = filepath.Dir(file)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cmd run %s: %s", file, err)
		}
	}

	if err := concatenateBundleJsons(config.bundle); err != nil {
		return fmt.Errorf("concat bundle %s json: %s", config.bundle, err)
	}
	return nil
}

func fixAssetPermissions() error {
	matches, err := glob(TestDirRoot, `.*\.o`,
		filterPackagesFn(filterPaths{
			paths:     []string{"pkg/ebpf/bytecode/build"},
			inclusive: true,
		}),
	)
	if err != nil {
		return fmt.Errorf("glob assets: %s", err)
	}

	for _, file := range matches {
		if err := os.Chown(file, 0, 0); err != nil {
			return fmt.Errorf("chown %s: %s", file, err)
		}
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := fixAssetPermissions(); err != nil {
		return err
	}

	if err := testPass(testConfig{
		bundle: "prebuilt",
		env: map[string]interface{}{
			"DD_ENABLE_RUNTIME_COMPILER": false,
			"DD_ENABLE_CO_RE":            false,
		},
		filterPackages: skipPrebuiltTests,
	}); err != nil {
		return err
	}
	if err := testPass(testConfig{
		bundle: "runtime",
		env: map[string]interface{}{
			"DD_ENABLE_RUNTIME_COMPILER":    true,
			"DD_ALLOW_PRECOMPILED_FALLBACK": false,
			"DD_ENABLE_CO_RE":               false,
		},
		filterPackages: runtimeCompiledTests,
	}); err != nil {
		return err
	}
	if err := testPass(testConfig{
		bundle: "co-re",
		env: map[string]interface{}{
			"DD_ENABLE_CO_RE":                    true,
			"DD_ENABLE_RUNTIME_COMPILER":         false,
			"DD_ALLOW_RUNTIME_COMPILED_FALLBACK": false,
			"DD_ALLOW_PRECOMPILED_FALLBACK":      false,
		},
		filterPackages: coreTests,
	}); err != nil {
		return err
	}
	if err := testPass(testConfig{
		bundle: "fentry",
		env: map[string]interface{}{
			"ECS_FARGATE":                   true,
			"DD_ENABLE_CO_RE":               true,
			"DD_ENABLE_RUNTIME_COMPILER":    false,
			"DD_ALLOW_PRECOMPILED_FALLBACK": false,
		},
		filterPackages: fentryTests,
	}); err != nil {
		return err
	}
	return nil
}
