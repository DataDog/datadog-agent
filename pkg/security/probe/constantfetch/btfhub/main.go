// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/probe/constantfetch"
	utilKernel "github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func main() {
	var archiveRootPath string
	var constantOutputPath string
	var sampling int

	flag.StringVar(&archiveRootPath, "archive-root", "", "Root path of BTFHub archive")
	flag.StringVar(&constantOutputPath, "output", "", "Output path for JSON constants")
	flag.IntVar(&sampling, "sampling", 1, "Sampling rate, take 1 over n elements")
	flag.Parse()

	twCollector := newTreeWalkCollector(sampling)

	if err := filepath.WalkDir(archiveRootPath, twCollector.treeWalkerBuilder(archiveRootPath)); err != nil {
		panic(err)
	}
	fmt.Println(len(twCollector.infos))

	output, err := json.MarshalIndent(twCollector.infos, "", "\t")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(constantOutputPath, output, 0644); err != nil {
		panic(err)
	}
}

type treeWalkCollector struct {
	infos    []constantfetch.BTFHubConstantsInfo
	counter  int
	sampling int
}

func newTreeWalkCollector(sampling int) *treeWalkCollector {
	return &treeWalkCollector{
		infos:    make([]constantfetch.BTFHubConstantsInfo, 0),
		counter:  0,
		sampling: sampling,
	}
}

func (c *treeWalkCollector) treeWalkerBuilder(prefix string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		c.counter++
		if c.counter != c.sampling {
			return nil
		}
		c.counter = 0

		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".tar.xz") {
			return nil
		}

		pathSuffix := strings.TrimPrefix(path, prefix)

		btfParts := strings.Split(pathSuffix, "/")
		if len(btfParts) != 4 {
			return fmt.Errorf("file has wront format: %s", pathSuffix)
		}

		distribution := btfParts[0]
		distribVersion := btfParts[1]
		arch := btfParts[2]
		unameRelease := strings.TrimSuffix(btfParts[3], ".btf.tar.xz")

		fmt.Println(path)

		constants, err := extractConstantsFromBTF(path)
		if err != nil {
			return err
		}

		c.infos = append(c.infos, constantfetch.BTFHubConstantsInfo{
			Distribution:   distribution,
			DistribVersion: distribVersion,
			Arch:           arch,
			UnameRelease:   unameRelease,
			Constants:      constants,
		})

		return err
	}
}

func extractConstantsFromBTF(archivePath string) (map[string]uint64, error) {
	tmpDir, err := os.MkdirTemp("", "extract-dir")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	extractCmd := exec.Command("tar", "xvf", archivePath, "-C", tmpDir)
	if err := extractCmd.Run(); err != nil {
		return nil, err
	}

	archiveFileName := path.Base(archivePath)
	btfFileName := strings.TrimSuffix(archiveFileName, ".tar.xz")
	btfPath := path.Join(tmpDir, btfFileName)

	releasePart := strings.Split(btfFileName, "-")[0]
	kvCode, err := utilKernel.ParseReleaseString(releasePart)
	if err != nil {
		return nil, err
	}
	kv := &kernel.Version{
		Code: kvCode,
	}

	fetcher := newConstantCollector(btfPath)

	return probe.GetOffsetConstantsFromFetcher(fetcher, kv)
}

type constantCollector struct {
	btfPath  string
	requests []constantRequest
}

func newConstantCollector(btfPath string) *constantCollector {
	return &constantCollector{
		btfPath:  btfPath,
		requests: make([]constantRequest, 0),
	}
}

var sizeRe = regexp.MustCompile(`size: (\d+), cachelines: \d+, members: \d+`)
var offsetRe = regexp.MustCompile(`/\*\s*(\d+)\s*\d+\s*\*/`)
var notFoundRe = regexp.MustCompile(`pahole: type '(\w+)' not found`)

type constantRequest struct {
	id                  string
	sizeof              bool
	typeName, fieldName string
}

func (cc *constantCollector) AppendSizeofRequest(id, typeName, headerName string) {
	cc.requests = append(cc.requests, constantRequest{
		id:       id,
		sizeof:   true,
		typeName: getActualTypeName(typeName),
	})
}

func (cc *constantCollector) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	cc.requests = append(cc.requests, constantRequest{
		id:        id,
		sizeof:    false,
		typeName:  getActualTypeName(typeName),
		fieldName: fieldName,
	})
}

func (cc *constantCollector) FinishAndGetResults() (map[string]uint64, error) {
	typeNames := make([]string, 0, len(cc.requests))
	for _, r := range cc.requests {
		typeNames = append(typeNames, r.typeName)
	}
	output, err := loopRunPahole(cc.btfPath, typeNames)
	if err != nil {
		exitErr := err.(*exec.ExitError)
		fmt.Println(string(exitErr.Stderr))
		return nil, err
	}

	perStruct := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(output))

	var currentTypeName string
	var currentTypeContent string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "struct ") || strings.HasPrefix(line, "enum ") {
			currentTypeName = getActualTypeName(strings.TrimSuffix(line, " {"))
			currentTypeContent = line + "\n"
		} else if strings.HasPrefix(line, "};") {
			perStruct[currentTypeName] = currentTypeContent + "};"
			currentTypeContent = ""
			currentTypeName = ""
		} else {
			currentTypeContent += line + "\n"
		}
	}

	pc := paholeCache{
		perStruct: perStruct,
	}
	constants := make(map[string]uint64)

	for _, r := range cc.requests {
		if r.sizeof {
			value := pc.parsePaholeOutput(r.typeName, func(line string) (uint64, bool) {
				if matches := sizeRe.FindStringSubmatch(line); len(matches) != 0 {
					size, err := strconv.ParseUint(matches[1], 10, 64)
					if err != nil {
						panic(err)
					}
					return size, true
				}
				return 0, false
			})
			constants[r.id] = value
		} else {
			value := pc.parsePaholeOutput(r.typeName, func(line string) (uint64, bool) {
				if strings.Contains(line, r.fieldName+";") || strings.Contains(line, r.fieldName+"[") {
					if matches := offsetRe.FindStringSubmatch(line); len(matches) != 0 {
						size, err := strconv.ParseUint(matches[1], 10, 64)
						if err != nil {
							panic(err)
						}
						return size, true
					}
				}
				return 0, false
			})
			constants[r.id] = value
		}
	}

	return constants, nil
}

func getActualTypeName(tn string) string {
	prefixes := []string{"struct", "enum"}
	for _, prefix := range prefixes {
		tn = strings.TrimPrefix(tn, prefix+" ")
	}
	return tn
}

type paholeCache struct {
	perStruct map[string]string
}

func (pc *paholeCache) parsePaholeOutput(tyName string, lineF func(string) (uint64, bool)) uint64 {
	scanner := bufio.NewScanner(strings.NewReader(pc.perStruct[tyName]))
	for scanner.Scan() {
		line := scanner.Text()
		value, ok := lineF(line)
		if ok {
			return value
		}
	}
	return constantfetch.ErrorSentinel
}

func loopRunPahole(btfPath string, typeNames []string) (string, error) {
	typeMap := make(map[string]bool)
	for _, ty := range typeNames {
		typeMap[ty] = true
	}

	for {
		paholeTyNames := make([]string, 0, len(typeMap))
		for k, ok := range typeMap {
			if ok {
				paholeTyNames = append(paholeTyNames, k)
			}
		}

		output, err := runPahole(btfPath, paholeTyNames)
		if err != nil {
			return "", err
		}
		scanner := bufio.NewScanner(strings.NewReader(output))
		hasError := false
		for scanner.Scan() {
			line := scanner.Text()
			if matches := notFoundRe.FindStringSubmatch(line); len(matches) != 0 {
				errorTy := matches[1]
				typeMap[errorTy] = false
				hasError = true
			}
		}

		if !hasError {
			return output, nil
		}
	}
}

func runPahole(btfPath string, typeNames []string) (string, error) {
	typeNamesArg := strings.Join(typeNames, ",")

	var btfArg string
	if btfPath != "" {
		btfArg = fmt.Sprintf("--btf_base=%s", btfPath)
	}

	cmd := exec.Command("pahole", typeNamesArg, btfArg)
	cmd.Stdin = os.Stdin
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
