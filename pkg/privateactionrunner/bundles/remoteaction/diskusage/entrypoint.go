package com_datadoghq_remoteaction_diskusage

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type DiskUsageBundle struct {
	actions map[string]types.Action
}

func NewDiskUsageBundle() *DiskUsageBundle {
	return &DiskUsageBundle{
		actions: map[string]types.Action{
			"analyze": NewAnalyzeHandler(),
		},
	}
}

func (b *DiskUsageBundle) GetAction(actionName string) types.Action {
	return b.actions[actionName]
}
