// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate accessors -tags windows -types-file model.go -output accessors_windows.go -field-handlers field_handlers_windows.go -field-accessors-output field_accessors_windows.go

// Package model holds model related files
package model

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// ValidateField validates the value of a field
func (m *Model) ValidateField(field eval.Field, fieldValue eval.FieldValue) error {
	if m.ExtraValidateFieldFnc != nil {
		return m.ExtraValidateFieldFnc(field, fieldValue)
	}

	return nil
}

// Event represents an event sent from the kernel
// genaccessors
type Event struct {
	BaseEvent

	// process events
	Exec ExecEvent `field:"exec" event:"exec"` // [7.27] [Process] A process was executed or forked
	Exit ExitEvent `field:"exit" event:"exit"` // [7.38] [Process] A process was terminated

	// FIM
	CreateNewFile CreateNewFileEvent `field:"create" event:"create"` // [7.52] [File] A file was created

	// Registries
	CreateRegistryKey   CreateRegistryKeyEvent   `field:"create_key;create" event:"create_key" `   // [7.52] [Registry] A registry key was created
	OpenRegistryKey     OpenRegistryKeyEvent     `field:"open_key;open" event:"open_key"`          // [7.52] [Registry] A registry key was opened
	SetRegistryKeyValue SetRegistryKeyValueEvent `field:"set_key_value;set" event:"set_key_value"` // [7.52] [Registry] A registry key value was set
	DeleteRegistryKey   DeleteRegistryKeyEvent   `field:"delete_key;delete" event:"delete_key"`    // [7.52] [Registry] A registry key was deleted
}

// FileEvent is the common file event type
type FileEvent struct {
	PathnameStr string `field:"path,handler:ResolveFilePath,opts:length" op_override:"eval.CaseInsensitiveCmp"`     // SECLDoc[path] Definition:`File's path` Example:`exec.file.path == "c:\cmd.bat"` Description:`Matches the execution of the file located at c:\cmd.bat`
	BasenameStr string `field:"name,handler:ResolveFileBasename,opts:length" op_override:"eval.CaseInsensitiveCmp"` // SECLDoc[name] Definition:`File's basename` Example:`exec.file.name == "cmd.bat"` Description:`Matches the execution of any file named cmd.bat.`
}

// RegistryEvent is the common registry event type
type RegistryEvent struct {
	KeyName string `field:"key_name,opts:length"`                                       // SECLDoc[key_name] Definition:`Registry's name`
	KeyPath string `field:"key_path,opts:length" op_override:"eval.CaseInsensitiveCmp"` // SECLDoc[key_path] Definition:`Registry's path`
}

// Process represents a process
type Process struct {
	PIDContext

	FileEvent FileEvent `field:"file"`

	ContainerID string `field:"container.id"` // SECLDoc[container.id] Definition:`Container ID`

	ExitTime time.Time `field:"exit_time,opts:getters_only"`
	ExecTime time.Time `field:"exec_time,opts:getters_only"`

	CreatedAt uint64 `field:"created_at,handler:ResolveProcessCreatedAt"` // SECLDoc[created_at] Definition:`Timestamp of the creation of the process`

	PPid uint32 `field:"ppid"` // SECLDoc[ppid] Definition:`Parent process ID`

	ArgsEntry *ArgsEntry `field:"-"`
	EnvsEntry *EnvsEntry `field:"-"`

	CmdLine         string `field:"cmdline,handler:ResolveProcessCmdLine,weight:200" op_override:"eval.CaseInsensitiveCmp"` // SECLDoc[cmdline] Definition:`Command line of the process` Example:`exec.cmdline == "-sV -p 22,53,110,143,4564 198.116.0-255.1-127"` Description:`Matches any process with these exact arguments.` Example:`exec.cmdline =~ "* -F * http*"` Description:`Matches any process that has the "-F" argument anywhere before an argument starting with "http".`
	CmdLineScrubbed string `field:"cmdline_scrubbed,handler:ResolveProcessCmdLineScrubbed,weight:500,opts:getters_only"`

	OwnerSidString string `field:"user_sid"`                 // SECLDoc[user_sid] Definition:`Sid of the user of the process`
	User           string `field:"user,handler:ResolveUser"` // SECLDoc[user] Definition:`User name`

	Envs []string `field:"envs,handler:ResolveProcessEnvs,weight:100"` // SECLDoc[envs] Definition:`Environment variable names of the process`
	Envp []string `field:"envp,handler:ResolveProcessEnvp,weight:100"` // SECLDoc[envp] Definition:`Environment variables of the process`                                                                                                                         // SECLDoc[envp] Definition:`Environment variables of the process`

	// cache version
	Variables               eval.Variables `field:"-"`
	ScrubbedCmdLineResolved bool           `field:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	*Process
}

// PIDContext holds the process context of an kernel event
type PIDContext struct {
	Pid uint32 `field:"pid"` // SECLDoc[pid] Definition:`Process ID of the process (also called thread group ID)`
}

// NetworkDeviceContext defines a network device context
type NetworkDeviceContext struct{}

// ExtraFieldHandlers handlers not hold by any field
type ExtraFieldHandlers interface {
	BaseExtraFieldHandlers
}

// FIM

// CreateNewFileEvent defines file creation
type CreateNewFileEvent struct {
	File FileEvent `field:"file"` // SECLDoc[file] Definition:`File Event`
}

// Registries

// CreateRegistryKeyEvent defines registry key creation
type CreateRegistryKeyEvent struct {
	Registry RegistryEvent `field:"registry"` // SECLDoc[registry] Definition:`Registry Event`
}

// OpenRegistryKeyEvent defines registry key opening
type OpenRegistryKeyEvent struct {
	Registry RegistryEvent `field:"registry"` // SECLDoc[registry] Definition:`Registry Event`
}

// SetRegistryKeyValueEvent defines the event of setting up a value of a registry key
type SetRegistryKeyValueEvent struct {
	Registry  RegistryEvent `field:"registry"`                                   // SECLDoc[registry] Definition:`Registry Event`
	ValueName string        `field:"value_name;registry.value_name,opts:length"` // SECLDoc[value_name] Definition:`Registry's value name`

}

// DeleteRegistryKeyEvent defines registry key deletion
type DeleteRegistryKeyEvent struct {
	Registry RegistryEvent `field:"registry"` // SECLDoc[registry] Definition:`Registry Event`
}
