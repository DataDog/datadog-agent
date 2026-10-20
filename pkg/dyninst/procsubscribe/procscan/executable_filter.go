// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && bpf

package procscan

import (
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var goELFSections = map[string]struct{}{
	".gosymtab":     {},
	".gopclntab":    {},
	".go.buildinfo": {},
}

// isGoELFBinary reports whether the file at path is an ELF binary built by the
// Go toolchain.
//
// A file that can't be opened or parsed is reported as an error because
// the answer is cached and an error shouldn't exclude it forever.
func isGoELFBinary(path string) (bool, error) {
	elfFile, err := object.OpenMMappingElfFile(path)
	if err != nil {
		return false, err
	}
	defer elfFile.Close()
	for _, section := range elfFile.SectionHeaders() {
		if _, ok := goELFSections[section.Name]; ok {
			return true, nil
		}
	}
	log.Tracef("procscan: %s has no Go sections", path)
	return false, nil
}
