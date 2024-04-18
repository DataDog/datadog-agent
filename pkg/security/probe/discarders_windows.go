// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package probe

import "github.com/DataDog/datadog-agent/pkg/security/secl/rules"

func init() {
	SupportedMultiDiscarder = []*rules.MultiDiscarder{
		{
			Entries: []rules.MultiDiscarderEntry{
				{
					Field:     "create.file.path",
					EventType: "create",
				},
			},
		},
		{
			Entries: []rules.MultiDiscarderEntry{
				{
					Field:     "create.file.name",
					EventType: "create",
				},
			},
		},
	}
}
