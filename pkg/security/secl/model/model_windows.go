// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:generate go run github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors -tags windows -types-file model.go -output accessors_windows.go -field-handlers field_handlers_windows.go

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
}

// FileEvent is the common file event type
type FileEvent struct {
	PathnameStr string `field:"path,handler:ResolveFilePath,opts:length"`     // SECLDoc[path] Definition:`File's path` Example:`exec.file.path == "c:\cmd.bat"` Description:`Matches the execution of the file located at c:\cmd.bat`
	BasenameStr string `field:"name,handler:ResolveFileBasename,opts:length"` // SECLDoc[name] Definition:`File's basename` Example:`exec.file.name == "cmd.bat"` Description:`Matches the execution of any file named cmd.bat.`
}

// Process represents a process
type Process struct {
	PIDContext

	FileEvent FileEvent `field:"file"`

	ContainerID string `field:"container.id"` // SECLDoc[container.id] Definition:`Container ID`

	SpanID  uint64 `field:"-"`
	TraceID uint64 `field:"-"`

	ExitTime time.Time `field:"-" json:"-"`
	ExecTime time.Time `field:"-" json:"-"`

	CreatedAt uint64 `field:"created_at,handler:ResolveProcessCreatedAt"` // SECLDoc[created_at] Definition:`Timestamp of the creation of the process`

	PPid uint32 `field:"ppid"` // SECLDoc[ppid] Definition:`Parent process ID`

	ArgsEntry *ArgsEntry `field:"-" json:"-"`
	EnvsEntry *EnvsEntry `field:"-" json:"-"`

	Argv0 string   `field:"argv0"`                                                                                                                    // SECLDoc[argv0] Definition:`First argument of the process`
	Args  string   `field:"args"`                                                                                                                     // SECLDoc[args] Definition:`Arguments of the process (as a string, excluding argv0)` Example:`exec.args == "-sV -p 22,53,110,143,4564 198.116.0-255.1-127"` Description:`Matches any process with these exact arguments.` Example:`exec.args =~ "* -F * http*"` Description:`Matches any process that has the "-F" argument anywhere before an argument starting with "http".`
	Argv  []string `field:"argv; args_flags,handler:ResolveProcessArgsFlags,opts:helper; args_options,handler:ResolveProcessArgsOptions,opts:helper"` // SECLDoc[argv] Definition:`Arguments of the process (as an array, excluding argv0)` Example:`exec.argv in ["127.0.0.1"]` Description:`Matches any process that has this IP address as one of its arguments.` SECLDoc[args_flags] Definition:`Flags in the process arguments` Example:`exec.args_flags in ["s"] && exec.args_flags in ["V"]` Description:`Matches any process with both "-s" and "-V" flags in its arguments. Also matches "-sV".` SECLDoc[args_options] Definition:`Argument of the process as options` Example:`exec.args_options in ["p=0-1024"]` Description:`Matches any process that has either "-p 0-1024" or "--p=0-1024" in its arguments.`                                                                                                                    // SECLDoc[envs] Definition:`Environment variable names of the process`
	Envs  []string `field:"envs,handler:ResolveProcessEnvs:100"`                                                                                      // SECLDoc[envs] Definition:`Environment variable names of the process`
	Envp  []string `field:"envp,handler:ResolveProcessEnvp:100"`                                                                                      // SECLDoc[envp] Definition:`Environment variables of the process`                                                                                                                         // SECLDoc[envp] Definition:`Environment variables of the process`

	// cache version
	ScrubbedArgvResolved bool           `field:"-" json:"-"`
	ScrubbedArgv         []string       `field:"-" json:"-"`
	Variables            eval.Variables `field:"-" json:"-"`
}

// ExecEvent represents a exec event
type ExecEvent struct {
	*Process
}

// PIDContext holds the process context of an kernel event
type PIDContext struct {
	Pid uint32 `field:"pid"` // SECLDoc[pid] Definition:`Process ID of the process (also called thread group ID)`
	Tid uint32 `field:"tid"` // SECLDoc[tid] Definition:`Thread ID of the thread`

	// NOTE: Used by the process cache entry. Should be reworked the decouple the different models
	NetNS     uint32 `field:"-"`
	IsKworker bool   `field:"-"` // SECLDoc[is_kworker] Definition:`Indicates whether the process is a kworker`
	ExecInode uint64 `field:"-"` // used to track exec and event loss
}

type NetworkDeviceContext struct{}
