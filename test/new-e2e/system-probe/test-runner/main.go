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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/sys/unix"
)

func init() {
	color.NoColor = false
}

const (
	Testsuite   = "testsuite"
	TestDirRoot = "/opt/system-probe-tests"
	GoTestSum   = "/go/bin/gotestsum"
)

var BaseEnv = map[string]interface{}{
	"DD_SYSTEM_PROBE_BPF_DIR":  filepath.Join(TestDirRoot, "pkg/ebpf/bytecode/build"),
	"DD_SYSTEM_PROBE_JAVA_DIR": filepath.Join(TestDirRoot, "pkg/network/java"),
}

var timeouts = map[*regexp.Regexp]time.Duration{
	regexp.MustCompile("pkg/network/protocols/http$"): 15 * time.Minute,
	regexp.MustCompile("pkg/network/tracer$"):         55 * time.Minute,
	regexp.MustCompile("pkg/network/usm$"):            30 * time.Minute,
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

func buildCommandArgs(junitPath string, jsonPath string, file string) []string {
	pkg := generatePackageName(file)
	junitfilePrefix := strings.ReplaceAll(pkg, "/", "-")
	xmlpath := filepath.Join(
		junitPath,
		fmt.Sprintf("%s.xml", junitfilePrefix),
	)
	jsonpath := filepath.Join(
		jsonPath,
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

// concatenateJsons combines all the test json output files into a single file.
func concatenateJsons(indir, outdir string) error {
	testJsonFile := filepath.Join(outdir, "out.json")
	matches, err := glob(indir, `.*\.json`, func(path string) bool { return true })
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

func testPass() error {
	matches, err := glob(TestDirRoot, Testsuite, func(path string) bool {
		return true
	})
	if err != nil {
		return fmt.Errorf("test glob: %s", err)
	}

	xmlPath := "/junit"
	jsonPath := "/pkgjson"
	jsonOutPath := "/testjson"

	dirs := []string{xmlPath, jsonPath, jsonOutPath}
	for _, d := range dirs {
		if err := os.RemoveAll(d); err != nil {
			return fmt.Errorf("failed to remove contents of %s: %w", d, err)
		}
		if _, err := os.Stat(d); errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(d, 0777); err != nil {
				return fmt.Errorf("failed to create directory %s", d)
			}
		}
	}

	for _, file := range matches {
		args := buildCommandArgs(xmlPath, jsonPath, file)
		cmd := exec.Command(GoTestSum, args...)

		cmd.Env = append(cmd.Environ(), mergeEnv(BaseEnv)...)
		cmd.Dir = filepath.Dir(file)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			// log but do not return error
			fmt.Fprintf(os.Stderr, "cmd run %s: %s", file, err)
		}
	}

	if err := concatenateJsons(jsonPath, jsonOutPath); err != nil {
		return fmt.Errorf("concat json: %s", err)
	}
	return nil
}

func fixAssetPermissions() error {
	matches, err := glob(TestDirRoot, `.*\.o`, func(path string) bool {
		return pathEmbedded(path, "pkg/ebpf/bytecode/build")
	})
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
		fmt.Fprintf(os.Stderr, "%s\n", color.RedString(err.Error()))
		os.Exit(1)
	}
}

func run() error {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return fmt.Errorf("error calling uname: %w", err)
	}
	fmt.Printf("running on: %s\n", unix.ByteSliceToString(uname.Release[:]))
	if err := fixAssetPermissions(); err != nil {
		return err
	}
	return testPass()
}
