// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cpu

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetContextSwitches retrieves the number of context switches for the current process.
// It returns an integer representing the count and an error if the retrieval fails.
func GetContextSwitches() (ctxSwitches int64, err error) {
	log.Debug("collecting ctx switches")
	procfsPath := "/proc"
	if pkgconfigsetup.Datadog().IsSet("procfs_path") {
		procfsPath = pkgconfigsetup.Datadog().GetString("procfs_path")
	}
	filePath := procfsPath + "/stat"
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for i := 0; scanner.Scan(); i++ {
		txt := scanner.Text()
		if strings.HasPrefix(txt, "ctxt") {
			elemts := strings.Split(txt, " ")
			ctxSwitches, err = strconv.ParseInt(elemts[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("%s in '%s' at line %d", err, filePath, i)
			}
			return ctxSwitches, nil
		}
	}
	return 0, errors.New("could not find the context switches in stat file")
}
