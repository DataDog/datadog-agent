// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func init() {
	SupportedMultiDiscarder = []*rules.MultiDiscarder{
		{
			Entries: []rules.MultiDiscarderEntry{
				{
					Field:     "create.file.device_path",
					EventType: model.CreateNewFileEventType,
				},
				{
					Field:     "rename.file.device_path",
					EventType: model.FileRenameEventType,
				},
				{
					Field:     "delete.file.device_path",
					EventType: model.DeleteFileEventType,
				},
				{
					Field:     "write.file.device_path",
					EventType: model.WriteFileEventType,
				},
			},
			FinalField:     "create.file.device_path",
			FinalEventType: model.CreateNewFileEventType,
		},
		{
			Entries: []rules.MultiDiscarderEntry{
				{
					Field:     "create.file.path",
					EventType: model.CreateNewFileEventType,
				},
				{
					Field:     "rename.file.path",
					EventType: model.FileRenameEventType,
				},
				{
					Field:     "delete.file.path",
					EventType: model.DeleteFileEventType,
				},
				{
					Field:     "write.file.path",
					EventType: model.WriteFileEventType,
				},
			},
			FinalField:     "create.file.path",
			FinalEventType: model.CreateNewFileEventType,
		},
		{
			Entries: []rules.MultiDiscarderEntry{
				{
					Field:     "create.file.name",
					EventType: model.CreateNewFileEventType,
				},
				{
					Field:     "rename.file.name",
					EventType: model.FileRenameEventType,
				},
				{
					Field:     "delete.file.name",
					EventType: model.DeleteFileEventType,
				},
				{
					Field:     "write.file.name",
					EventType: model.WriteFileEventType,
				},
			},
			FinalField:     "create.file.name",
			FinalEventType: model.CreateNewFileEventType,
		},
	}
}
