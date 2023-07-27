// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package detectors

import (
	"debug/elf"
	"fmt"

	dderrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
)

type GoDetector struct{}

// DetectLanguage allows for detecting if a process is a go process, and it's version.
// Note that currently the GoDetector only returns non-retriable errors. It's failure modes are:
// - Invalid permissions. In this case we should warn.
func (GoDetector) DetectLanguage(pid int) (languagemodels.Language, error) {
	exePath := getExePath(pid)

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
