// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
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
	archivePath := os.Args[1]
	twCollector := TreeWalkCollector{}

	if err := filepath.WalkDir(archivePath, twCollector.treeWalkerBuilder(archivePath)); err != nil {
		panic(err)
	}
	fmt.Println(len(twCollector.infos))
	fmt.Println(twCollector.infos[:5])
}

type TreeWalkCollector struct {
	infos []ConstantsInfo
}

func NewTreeWalkCollector() *TreeWalkCollector {
	return &TreeWalkCollector{
		infos: make([]ConstantsInfo, 0),
	}
}

func (c *TreeWalkCollector) treeWalkerBuilder(prefix string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
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

		fmt.Println(path)

		constants, err := extractConstantsFromBTF(path)
		if err != nil {
			return err
		}

		c.infos = append(c.infos, ConstantsInfo{
			distribution:   distribution,
			distribVersion: distribVersion,
			arch:           arch,
			constants:      constants,
			err:            err,
		})

		return err
	}
}

type ConstantsInfo struct {
	distribution   string
	distribVersion string
	arch           string
	constants      map[string]uint64
	err            error
}

func extractConstantsFromBTF(archivePath string) (map[string]uint64, error) {
	tmpDir, err := os.MkdirTemp("", "extract-dir")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	extractCmd := exec.Command("tar", "xvf", archivePath, "-C", tmpDir)
	if err := extractCmd.Run(); err != nil {
		panic(err)
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

	fetcher := NewConstantCollector(btfPath)

	return probe.GetOffsetConstantsFromFetcher(fetcher, kv)
}

type ConstantCollector struct {
	constants   map[string]uint64
	paholeCache PaholeCache
}

func NewConstantCollector(btfPath string) *ConstantCollector {
	return &ConstantCollector{
		constants: make(map[string]uint64),
		paholeCache: PaholeCache{
			btfPath: btfPath,
		},
	}
}

var sizeRe = regexp.MustCompile(`size: (\d+), cachelines: \d+, members: \d+`)
var offsetRe = regexp.MustCompile(`/\*\s*(\d+)\s*\d+\s*\*/`)

func (cc *ConstantCollector) AppendSizeofRequest(id, typeName, headerName string) {
	value := cc.paholeCache.parsePaholeOutput(getActualTypeName(typeName), func(line string) (uint64, bool) {
		if matches := sizeRe.FindStringSubmatch(line); len(matches) != 0 {
			size, err := strconv.ParseUint(matches[1], 10, 64)
			if err != nil {
				panic(err)
			}
			return size, true
		}
		return 0, false
	})
	cc.constants[id] = value
}

func (cc *ConstantCollector) AppendOffsetofRequest(id, typeName, fieldName, headerName string) {
	value := cc.paholeCache.parsePaholeOutput(getActualTypeName(typeName), func(line string) (uint64, bool) {
		if strings.Contains(line, fieldName) {
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
	cc.constants[id] = value
}

func (c *ConstantCollector) FinishAndGetResults() (map[string]uint64, error) {
	return c.constants, nil
}

func getActualTypeName(tn string) string {
	prefixes := []string{"struct", "enum"}
	for _, prefix := range prefixes {
		tn = strings.TrimPrefix(tn, prefix+" ")
	}
	return tn
}

type PaholeCache struct {
	btfPath string
	cache   map[string]string
}

func (pc *PaholeCache) parsePaholeOutput(tyName string, lineF func(string) (uint64, bool)) uint64 {
	var output string
	if value, ok := pc.cache[tyName]; ok {
		output = value
	} else {
		var btfArg string
		if pc.btfPath != "" {
			btfArg = fmt.Sprintf("--btf_base=%s", pc.btfPath)
		}
		cmd := exec.Command("pahole", tyName, btfArg)
		cmd.Stdin = os.Stdin
		cmdOutput, err := cmd.Output()
		if err != nil {
			exitErr := err.(*exec.ExitError)
			fmt.Println(string(exitErr.Stderr))
			panic(err)
		}
		output = string(cmdOutput)
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		value, ok := lineF(line)
		if ok {
			return value
		}
	}
	return constantfetch.ErrorSentinel
}
