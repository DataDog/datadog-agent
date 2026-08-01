// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Code generated - DO NOT EDIT.

//go:build windows

package seclcel

import (
	"github.com/google/cel-go/common/types"
)

// modelShapes holds every CEL object type the SECL field namespace is made of,
// keyed by type name and then by member name.
//
// The types come from a trie over the SECL field *names* rather than from the Go
// structs behind them: `connect.addr` draws its members from a nested
// IPPortContext and from flat Go fields tagged `addr.hostname` and `addr.family`,
// so no Go type describes it.
//
// There is one type per path rather than one per member set, so a member always
// denotes exactly one SECL field — see modelPaths.
var modelShapes = map[string]map[string]*types.Type{
	// secl.ChangePermission is the type of change_permission.
	"secl.ChangePermission": {
		"new_sd":      types.StringType,
		"old_sd":      types.StringType,
		"path":        types.StringType,
		"type":        types.StringType,
		"user_domain": types.StringType,
		"username":    types.StringType,
	},
	// secl.Create is the type of create.
	"secl.Create": {
		"file":     types.NewObjectType("secl.CreateFile"),
		"registry": types.NewObjectType("secl.CreateRegistry"),
	},
	// secl.CreateFile is the type of create.file.
	"secl.CreateFile": {
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
	// secl.CreateKey is the type of create_key.
	"secl.CreateKey": {
		"registry": types.NewObjectType("secl.CreateKeyRegistry"),
	},
	// secl.CreateKeyRegistry is the type of create_key.registry.
	"secl.CreateKeyRegistry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.CreateRegistry is the type of create.registry.
	"secl.CreateRegistry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.Delete is the type of delete.
	"secl.Delete": {
		"file":     types.NewObjectType("secl.DeleteFile"),
		"registry": types.NewObjectType("secl.DeleteRegistry"),
	},
	// secl.DeleteFile is the type of delete.file.
	"secl.DeleteFile": {
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
	// secl.DeleteKey is the type of delete_key.
	"secl.DeleteKey": {
		"registry": types.NewObjectType("secl.DeleteKeyRegistry"),
	},
	// secl.DeleteKeyRegistry is the type of delete_key.registry.
	"secl.DeleteKeyRegistry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.DeleteRegistry is the type of delete.registry.
	"secl.DeleteRegistry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.Event is the type of event.
	"secl.Event": {
		"hostname":  types.StringType,
		"origin":    types.StringType,
		"os":        types.StringType,
		"rule":      types.NewObjectType("secl.EventRule"),
		"service":   types.StringType,
		"source":    types.StringType,
		"timestamp": types.IntType,
	},
	// secl.EventRule is the type of event.rule.
	"secl.EventRule": {
		"tags": types.NewListType(types.StringType),
	},
	// secl.Exec is the type of exec.
	"secl.Exec": {
		"cmdline":    types.StringType,
		"container":  types.NewObjectType("secl.ExecContainer"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.ExecFile"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.ExecContainer is the type of exec.container.
	"secl.ExecContainer": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.ExecFile is the type of exec.file.
	"secl.ExecFile": {
		"extension": types.StringType,
		"name":      types.StringType,
		"path":      types.StringType,
	},
	// secl.Exit is the type of exit.
	"secl.Exit": {
		"cause":      types.IntType,
		"cmdline":    types.StringType,
		"code":       types.IntType,
		"container":  types.NewObjectType("secl.ExitContainer"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.ExitFile"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.ExitContainer is the type of exit.container.
	"secl.ExitContainer": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.ExitFile is the type of exit.file.
	"secl.ExitFile": {
		"extension": types.StringType,
		"name":      types.StringType,
		"path":      types.StringType,
	},
	// secl.Open is the type of open.
	"secl.Open": {
		"registry": types.NewObjectType("secl.OpenRegistry"),
	},
	// secl.OpenKey is the type of open_key.
	"secl.OpenKey": {
		"registry": types.NewObjectType("secl.OpenKeyRegistry"),
	},
	// secl.OpenKeyRegistry is the type of open_key.registry.
	"secl.OpenKeyRegistry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.OpenRegistry is the type of open.registry.
	"secl.OpenRegistry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.Process is the type of process.
	"secl.Process": {
		"ancestors":  types.NewListType(types.NewObjectType("secl.ProcessAncestors")),
		"cmdline":    types.StringType,
		"container":  types.NewObjectType("secl.ProcessContainer"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.ProcessFile"),
		"parent":     types.NewObjectType("secl.ProcessParent"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.ProcessAncestors is the type of process.ancestors.
	"secl.ProcessAncestors": {
		"cmdline":    types.StringType,
		"container":  types.NewObjectType("secl.ProcessAncestorsContainer"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.ProcessAncestorsFile"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.ProcessAncestorsContainer is the type of process.ancestors.container.
	"secl.ProcessAncestorsContainer": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.ProcessAncestorsFile is the type of process.ancestors.file.
	"secl.ProcessAncestorsFile": {
		"extension": types.StringType,
		"name":      types.StringType,
		"path":      types.StringType,
	},
	// secl.ProcessContainer is the type of process.container.
	"secl.ProcessContainer": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.ProcessFile is the type of process.file.
	"secl.ProcessFile": {
		"extension": types.StringType,
		"name":      types.StringType,
		"path":      types.StringType,
	},
	// secl.ProcessParent is the type of process.parent.
	"secl.ProcessParent": {
		"cmdline":    types.StringType,
		"container":  types.NewObjectType("secl.ProcessParentContainer"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.ProcessParentFile"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.ProcessParentContainer is the type of process.parent.container.
	"secl.ProcessParentContainer": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.ProcessParentFile is the type of process.parent.file.
	"secl.ProcessParentFile": {
		"extension": types.StringType,
		"name":      types.StringType,
		"path":      types.StringType,
	},
	// secl.Rename is the type of rename.
	"secl.Rename": {
		"file": types.NewObjectType("secl.RenameFile"),
	},
	// secl.RenameFile is the type of rename.file.
	"secl.RenameFile": {
		"destination": types.NewObjectType("secl.RenameFileDestination"),
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
	// secl.RenameFileDestination is the type of rename.file.destination.
	"secl.RenameFileDestination": {
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
	// secl.Set is the type of set.
	"secl.Set": {
		"registry":   types.NewObjectType("secl.SetRegistry"),
		"value_name": types.StringType,
	},
	// secl.SetKeyValue is the type of set_key_value.
	"secl.SetKeyValue": {
		"registry":   types.NewObjectType("secl.SetKeyValueRegistry"),
		"value_name": types.StringType,
	},
	// secl.SetKeyValueRegistry is the type of set_key_value.registry.
	"secl.SetKeyValueRegistry": {
		"key_name":   types.StringType,
		"key_path":   types.StringType,
		"value_name": types.StringType,
	},
	// secl.SetRegistry is the type of set.registry.
	"secl.SetRegistry": {
		"key_name":   types.StringType,
		"key_path":   types.StringType,
		"value_name": types.StringType,
	},
	// secl.Write is the type of write.
	"secl.Write": {
		"file": types.NewObjectType("secl.WriteFile"),
	},
	// secl.WriteFile is the type of write.file.
	"secl.WriteFile": {
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
}

// modelPaths gives the SECL path each object type describes.
//
// It is what makes a member select resolvable at planning time: joining the type's
// path with the member name gives the SECL field, and so the reader for it, once
// per rule rather than once per read.
var modelPaths = map[string]string{
	"secl.ChangePermission":          "change_permission",
	"secl.Create":                    "create",
	"secl.CreateFile":                "create.file",
	"secl.CreateKey":                 "create_key",
	"secl.CreateKeyRegistry":         "create_key.registry",
	"secl.CreateRegistry":            "create.registry",
	"secl.Delete":                    "delete",
	"secl.DeleteFile":                "delete.file",
	"secl.DeleteKey":                 "delete_key",
	"secl.DeleteKeyRegistry":         "delete_key.registry",
	"secl.DeleteRegistry":            "delete.registry",
	"secl.Event":                     "event",
	"secl.EventRule":                 "event.rule",
	"secl.Exec":                      "exec",
	"secl.ExecContainer":             "exec.container",
	"secl.ExecFile":                  "exec.file",
	"secl.Exit":                      "exit",
	"secl.ExitContainer":             "exit.container",
	"secl.ExitFile":                  "exit.file",
	"secl.Open":                      "open",
	"secl.OpenKey":                   "open_key",
	"secl.OpenKeyRegistry":           "open_key.registry",
	"secl.OpenRegistry":              "open.registry",
	"secl.Process":                   "process",
	"secl.ProcessAncestors":          "process.ancestors",
	"secl.ProcessAncestorsContainer": "process.ancestors.container",
	"secl.ProcessAncestorsFile":      "process.ancestors.file",
	"secl.ProcessContainer":          "process.container",
	"secl.ProcessFile":               "process.file",
	"secl.ProcessParent":             "process.parent",
	"secl.ProcessParentContainer":    "process.parent.container",
	"secl.ProcessParentFile":         "process.parent.file",
	"secl.Rename":                    "rename",
	"secl.RenameFile":                "rename.file",
	"secl.RenameFileDestination":     "rename.file.destination",
	"secl.Set":                       "set",
	"secl.SetKeyValue":               "set_key_value",
	"secl.SetKeyValueRegistry":       "set_key_value.registry",
	"secl.SetRegistry":               "set.registry",
	"secl.Write":                     "write",
	"secl.WriteFile":                 "write.file",
}

// modelRoots holds the top level segments of the SECL field namespace, which are
// the names a CEL environment declares as variables.
var modelRoots = map[string]*types.Type{
	"change_permission": types.NewObjectType("secl.ChangePermission"),
	"create":            types.NewObjectType("secl.Create"),
	"create_key":        types.NewObjectType("secl.CreateKey"),
	"delete":            types.NewObjectType("secl.Delete"),
	"delete_key":        types.NewObjectType("secl.DeleteKey"),
	"event":             types.NewObjectType("secl.Event"),
	"exec":              types.NewObjectType("secl.Exec"),
	"exit":              types.NewObjectType("secl.Exit"),
	"open":              types.NewObjectType("secl.Open"),
	"open_key":          types.NewObjectType("secl.OpenKey"),
	"process":           types.NewObjectType("secl.Process"),
	"rename":            types.NewObjectType("secl.Rename"),
	"set":               types.NewObjectType("secl.Set"),
	"set_key_value":     types.NewObjectType("secl.SetKeyValue"),
	"write":             types.NewObjectType("secl.Write"),
}
