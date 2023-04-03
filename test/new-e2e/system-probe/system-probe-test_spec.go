package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	Testsuite   = "testsuite"
	TestDirRoot = "/opt/system-probe-tests"
	Sudo        = "sudo"
)

var BaseEnv = map[string]interface{}{
	"DD_SYSTEM_PROBE_BPF_DIR": filepath.Join("/opt/system-probe-tests", "pkg/ebpf/bytecode/build"),
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

func pathEmbedded(fullPath, embedded string) bool {
	normalized := fmt.Sprintf("/%s/",
		strings.TrimRight(
			strings.TrimLeft(embedded, "/"),
			"/",
		),
	)

	return strings.Contains(fullPath, normalized)
}

func glob(dir, filePattern string, filter filterPaths) ([]string, error) {
	var matches []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		present, err := regexp.Match(filePattern, []byte(d.Name()))
		if err != nil {
			return err
		}

		if d.IsDir() || !present {
			return nil
		}
		for _, p := range filter.paths {
			if pathEmbedded(path, p) && filter.inclusive {
				matches = append(matches, path)
			} else if !pathEmbedded(path, p) && !filter.inclusive {
				matches = append(matches, path)
			}
		}
		return nil
	})
	if err != nil {
		return []string{}, err
	}

	return matches, nil
}

func generatePacakgeName(file string) string {
	pkg := strings.TrimLeft(
		strings.TrimRight(
			strings.TrimPrefix(
				strings.TrimSuffix(file, Testsuite),
				TestDirRoot,
			),
			"/"),
		"/")

	return pkg
}

func buildCommandArgs(file, bundle string) []string {
	pkg := generatePacakgeName(file)
	junitfilePrefix := strings.ReplaceAll(pkg, "/", "-")
	xmlpath := filepath.Join(
		"/", "tmp", bundle,
		fmt.Sprintf("%s.xml", junitfilePrefix),
	)
	jsonpath := filepath.Join(
		"/", "tmp", bundle,
		fmt.Sprintf("%s.json", junitfilePrefix),
	)
	args := []string{
		"-E",
		"/go/bin/gotestsum",
		"--format", "dots",
		"--junitfile", xmlpath,
		"--jsonfile", jsonpath,
		"--raw-command", "--",
		"/go/bin/test2json", "-t", "-p", pkg, file, "-test.v", "-test.count=1",
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

func runCommandAndStreamOutput(cmd *exec.Cmd, commandOutput io.Reader) error {
	go func() {
		scanner := bufio.NewScanner(commandOutput)
		for scanner.Scan() {
			_, _ = os.Stdout.Write([]byte(scanner.Text() + "\n"))
		}
	}()

	return cmd.Run()
}

func testPass(config testConfig) error {
	matches, err := glob(TestDirRoot, Testsuite, config.filterPackages)
	if err != nil {
		return err
	}

	for _, file := range matches {
		args := buildCommandArgs(file, config.bundle)
		fmt.Println(args)
		cmd := exec.Command(Sudo, args...)

		r, w := io.Pipe()
		cmd.Env = mergeEnv(config.env, BaseEnv)
		cmd.Dir = filepath.Dir(file)
		cmd.Stdout = w
		cmd.Stderr = w

		if err := runCommandAndStreamOutput(cmd, r); err != nil {
			return err
		}
	}

	return nil
}

func fixAssetPermissions() error {
	matches, err := glob(TestDirRoot, `.*\.o`, filterPaths{
		paths:     []string{"pkg/ebpf/bytecode/build"},
		inclusive: true,
	})
	if err != nil {
		return err
	}

	for _, file := range matches {
		if err := os.Chown(file, 0, 0); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	if err := fixAssetPermissions(); err != nil {
		log.Fatal(err)
	}

	if err := testPass(testConfig{
		bundle: "prebuilt",
		env: map[string]interface{}{
			"DD_ENABLE_RUNTIME_COMPILER": false,
			"DD_ENABLE_CO_RE":            false,
		},
		filterPackages: skipPrebuiltTests,
	}); err != nil {
		log.Fatal(err)
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
		log.Fatal(err)
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
		log.Fatal(err)
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
		log.Fatal(err)
	}
}
