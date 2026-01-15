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
	GeneratedAt string         `json:"generated_at"`
	Modules     []LoadedModule `json:"modules"`
}

// LoadedModule describes a single loaded module (DLL).
type LoadedModule struct {
	ProcessName      string `json:"process_name"`
	ProcessPID       int    `json:"process_pid"`
	DLLName          string `json:"dll_name"`
	DLLPath          string `json:"dll_path"`
	FileTimestamp    string `json:"file_timestamp,omitempty"`
	CompanyName      string `json:"company_name,omitempty"`
	ProductName      string `json:"product_name,omitempty"`
	FileVersion      string `json:"file_version,omitempty"`
	ProductVersion   string `json:"product_version,omitempty"`
	OriginalFilename string `json:"original_filename,omitempty"`
	InternalName     string `json:"internal_name,omitempty"`
	Size             int64  `json:"size_bytes,omitempty"`
	Perms            string `json:"perms,omitempty"`
}

// ListLoadedModulesReportJSON returns a JSON payload describing DLLs loaded by the current agent process.
func ListLoadedModulesReportJSON() ([]byte, error) {
	files, err := ListOpenFilesFromSelf()
	if err != nil {
		return nil, err
	}

	exePath, _ := os.Executable()
	procName := filepath.Base(exePath)
	pid := os.Getpid()

	report := LoadedModulesReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	for _, f := range files {
		modPath := f.Name
		modName := filepath.Base(modPath)

		var ts string
		var size int64
		var perms string
		if fi, err := os.Stat(modPath); err == nil {
			size = fi.Size()
			perms = fi.Mode().Perm().String()
			ts = fi.ModTime().UTC().Format(time.RFC3339)
		}

		verInfo, _ := winutil.GetFileVersionInfoStrings(modPath)

		report.Modules = append(report.Modules, LoadedModule{
			ProcessName:      procName,
			ProcessPID:       pid,
			DLLName:          modName,
			DLLPath:          modPath,
			FileTimestamp:    ts,
			CompanyName:      verInfo.CompanyName,
			ProductName:      verInfo.ProductName,
			FileVersion:      verInfo.FileVersion,
			ProductVersion:   verInfo.ProductVersion,
			OriginalFilename: verInfo.OriginalFilename,
			InternalName:     verInfo.InternalName,
			Size:             size,
			Perms:            perms,
		})
	}

	return json.MarshalIndent(report, "", "  ")
}
