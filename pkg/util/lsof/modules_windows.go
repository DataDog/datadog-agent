// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package lsof

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// LoadedModulesReport is the JSON structure written to the flare as agent_loaded_modules.json.
type LoadedModulesReport struct {
	GeneratedAt string         `json:"generated_at"`           // RFC3339 UTC
	ProcessName string         `json:"process_name,omitempty"` // same for all entries
	ProcessPID  int            `json:"process_pid,omitempty"`  // same for all entries
	Modules     []LoadedModule `json:"modules"`
}

// LoadedModule describes a single loaded module (DLL).
type LoadedModule struct {
	DLLPath          string `json:"dll_path"`
	BuildTimestamp   string `json:"build_timestamp,omitempty"` // PE COFF TimeDateStamp
	FileTimestamp    string `json:"file_timestamp,omitempty"`  // on-disk modtime
	CompanyName      string `json:"company_name,omitempty"`
	ProductName      string `json:"product_name,omitempty"`
	FileVersion      string `json:"file_version,omitempty"`
	ProductVersion   string `json:"product_version,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	InternalName     string `json:"internal_name,omitempty"`
	SizeBytes        int64  `json:"size_bytes,omitempty"` // on-disk size
}

// ListLoadedModulesReportJSON returns a JSON payload describing DLLs loaded by the current agent process.
func ListLoadedModulesReportJSON() ([]byte, error) {
	// Reuse open-file enumeration to get module file paths for the current process.
	openFiles, err := ListOpenFilesFromSelf()
	if err != nil {
		return nil, err
	}

	exePath, _ := os.Executable()
	procName := filepath.Base(exePath)
	pid := os.Getpid()

	report := LoadedModulesReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339), // matches other flare artifacts
		ProcessName: procName,
		ProcessPID:  pid,
	}

	for _, f := range openFiles {
		modPath := f.Name

		var buildTS string
		if ts, err := winutil.GetPEBuildTimestamp(modPath); err == nil && !ts.IsZero() {
			buildTS = ts.Format(time.RFC3339)
		}

		var ts string
		var size int64
		if fi, err := os.Stat(modPath); err == nil {
			size = fi.Size()
			ts = fi.ModTime().UTC().Format(time.RFC3339)
		}

		verInfo, _ := winutil.GetFileVersionInfoStrings(modPath)

		report.Modules = append(report.Modules, LoadedModule{
			DLLPath:          modPath,
			BuildTimestamp:   buildTS,
			FileTimestamp:    ts,
			CompanyName:      verInfo.CompanyName,
			ProductName:      verInfo.ProductName,
			FileVersion:      verInfo.FileVersion,
			ProductVersion:   verInfo.ProductVersion,
			OriginalFilename: verInfo.OriginalFilename,
			InternalName:     verInfo.InternalName,
			SizeBytes:        size,
		})
	}

	return json.MarshalIndent(report, "", "  ")
}
