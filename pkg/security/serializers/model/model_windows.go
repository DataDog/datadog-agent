//go:generate easyjson -gen_build_flags=-mod=mod -gen_build_goos=$GEN_GOOS -no_std_marshalers -build_tags windows_tmp $GOFILE
//go:generate easyjson -gen_build_flags=-mod=mod -gen_build_goos=$GEN_GOOS -no_std_marshalers -build_tags windows_tmp -output_filename model_base_windows_easyjson.go model_base.go

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "github.com/DataDog/datadog-agent/pkg/security/utils"

// FileSerializer serializes a file to JSON
// easyjson:json
type FileSerializer struct {
	// File path
	Path string `json:"path,omitempty"`
	// File basename
	Name string `json:"name,omitempty"`
}

// ProcessSerializer serializes a process to JSON
// easyjson:json
type ProcessSerializer struct {
	// Process ID
	Pid uint32 `json:"pid,omitempty"`
	// Parent Process ID
	PPid *uint32 `json:"ppid,omitempty"`
	// Exec time of the process
	ExecTime *utils.EasyjsonTime `json:"exec_time,omitempty"`
	// Exit time of the process
	ExitTime *utils.EasyjsonTime `json:"exit_time,omitempty"`
	// File information of the executable
	Executable *FileSerializer `json:"executable,omitempty"`
	// Container context
	Container *ContainerContextSerializer `json:"container,omitempty"`
	// Command line arguments
	Args []string `json:"args,omitempty"`
	// Environment variables of the process
	Envs []string `json:"envs,omitempty"`
	// Process source
	Source string `json:"source,omitempty"`
}

// FileEventSerializer serializes a file event to JSON
// easyjson:json
type FileEventSerializer struct {
	FileSerializer
}

// NetworkDeviceSerializer serializes the network device context to JSON
// easyjson:json
type NetworkDeviceSerializer struct{}

// EventSerializer serializes an event to JSON
// easyjson:json
type EventSerializer struct {
	*BaseEventSerializer `json:"evt,omitempty"`
}
