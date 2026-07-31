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
// so no Go type describes it. One type is shared by every path exposing the same
// members, which is why a member carries no field name: the runtime composes the
// flat SECL name by joining the path it was reached through.
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
	// secl.Container is the type of exec.container, exit.container, process.ancestors.container, process.container and 1 more.
	"secl.Container": {
		"created_at": types.IntType,
		"id":         types.StringType,
		"tags":       types.NewListType(types.StringType),
	},
	// secl.Create is the type of create, delete.
	"secl.Create": {
		"file":     types.NewObjectType("secl.CreateFile"),
		"registry": types.NewObjectType("secl.Registry"),
	},
	// secl.CreateFile is the type of create.file, delete.file, rename.file.destination, write.file.
	"secl.CreateFile": {
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
	// secl.CreateKey is the type of create_key, delete_key, open, open_key.
	"secl.CreateKey": {
		"registry": types.NewObjectType("secl.Registry"),
	},
	// secl.Event is the type of event.
	"secl.Event": {
		"hostname":  types.StringType,
		"origin":    types.StringType,
		"os":        types.StringType,
		"rule":      types.NewObjectType("secl.Rule"),
		"service":   types.StringType,
		"source":    types.StringType,
		"timestamp": types.IntType,
	},
	// secl.Exec is the type of exec, process.ancestors, process.parent.
	"secl.Exec": {
		"cmdline":    types.StringType,
		"container":  types.NewObjectType("secl.Container"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.File"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.Exit is the type of exit.
	"secl.Exit": {
		"cause":      types.IntType,
		"cmdline":    types.StringType,
		"code":       types.IntType,
		"container":  types.NewObjectType("secl.Container"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.File"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.File is the type of exec.file, exit.file, process.ancestors.file, process.file and 1 more.
	"secl.File": {
		"extension": types.StringType,
		"name":      types.StringType,
		"path":      types.StringType,
	},
	// secl.Process is the type of process.
	"secl.Process": {
		"ancestors":  types.NewListType(types.NewObjectType("secl.Exec")),
		"cmdline":    types.StringType,
		"container":  types.NewObjectType("secl.Container"),
		"created_at": types.IntType,
		"envp":       types.NewListType(types.StringType),
		"envs":       types.NewListType(types.StringType),
		"file":       types.NewObjectType("secl.File"),
		"parent":     types.NewObjectType("secl.Exec"),
		"pid":        types.IntType,
		"ppid":       types.IntType,
		"user":       types.StringType,
		"user_sid":   types.StringType,
	},
	// secl.Registry is the type of create.registry, create_key.registry, delete.registry, delete_key.registry and 2 more.
	"secl.Registry": {
		"key_name": types.StringType,
		"key_path": types.StringType,
	},
	// secl.Rename is the type of rename.
	"secl.Rename": {
		"file": types.NewObjectType("secl.RenameFile"),
	},
	// secl.RenameFile is the type of rename.file.
	"secl.RenameFile": {
		"destination": types.NewObjectType("secl.CreateFile"),
		"device_path": types.StringType,
		"extension":   types.StringType,
		"name":        types.StringType,
		"path":        types.StringType,
	},
	// secl.Rule is the type of event.rule.
	"secl.Rule": {
		"tags": types.NewListType(types.StringType),
	},
	// secl.Set is the type of set, set_key_value.
	"secl.Set": {
		"registry":   types.NewObjectType("secl.SetRegistry"),
		"value_name": types.StringType,
	},
	// secl.SetRegistry is the type of set.registry, set_key_value.registry.
	"secl.SetRegistry": {
		"key_name":   types.StringType,
		"key_path":   types.StringType,
		"value_name": types.StringType,
	},
	// secl.Write is the type of write.
	"secl.Write": {
		"file": types.NewObjectType("secl.CreateFile"),
	},
}

// modelRoots holds the top level segments of the SECL field namespace, which are
// the names a CEL environment declares as variables.
var modelRoots = map[string]*types.Type{
	"change_permission": types.NewObjectType("secl.ChangePermission"),
	"create":            types.NewObjectType("secl.Create"),
	"create_key":        types.NewObjectType("secl.CreateKey"),
	"delete":            types.NewObjectType("secl.Create"),
	"delete_key":        types.NewObjectType("secl.CreateKey"),
	"event":             types.NewObjectType("secl.Event"),
	"exec":              types.NewObjectType("secl.Exec"),
	"exit":              types.NewObjectType("secl.Exit"),
	"open":              types.NewObjectType("secl.CreateKey"),
	"open_key":          types.NewObjectType("secl.CreateKey"),
	"process":           types.NewObjectType("secl.Process"),
	"rename":            types.NewObjectType("secl.Rename"),
	"set":               types.NewObjectType("secl.Set"),
	"set_key_value":     types.NewObjectType("secl.Set"),
	"write":             types.NewObjectType("secl.Write"),
}
