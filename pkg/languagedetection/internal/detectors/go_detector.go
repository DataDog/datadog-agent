// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package detectors

import (
	"debug/elf"
	"fmt"
	"path"
	"strconv"

	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

//nolint:revive // TODO(PROC) Fix revive linter
type GoDetector struct {
	hostProc string
}

//nolint:revive // TODO(PROC) Fix revive linter
func NewGoDetector() GoDetector {
	return GoDetector{hostProc: kernel.ProcFSRoot()}
}

// DetectLanguage allows for detecting if a process is a go process, and its version.
// Note that currently the goDetector only returns non-retriable errors since in all cases we will not be able to detect the language.
// Scenarios in which we can return an error:
//   - Program exits early, and we fail to call `elf.Open`. Note that in the future it may be possible to lock the directory using a system call.
//   - Program is not a go binary, or has build tags stripped out. In this case we return a `dderrors.NotFound`.
func (d GoDetector) DetectLanguage(process languagemodels.Process) (languagemodels.Language, error) {
	exePath := d.getHostProc(process.GetPid())

	bin, err := elf.Open(exePath)
	if err != nil {
		return languagemodels.Language{}, fmt.Errorf("open: %v", err)
	}
	defer bin.Close()

	vers, err := binversion.ReadElfBuildInfo(bin)
	if err != nil {
		return languagemodels.Language{}, dderrors.NewNotFound("go buildinf tags")
	}

	return languagemodels.Language{
		Name:    languagemodels.Go,
		Version: vers,
	}, nil
}

func (d GoDetector) getHostProc(pid int32) string {
	return path.Join(d.hostProc, strconv.FormatInt(int64(pid), 10), "exe")
}
