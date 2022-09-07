package net

import model "github.com/DataDog/agent-payload/v5/process"

type SysProbeUtil interface {
	GetConnections(clientID string) (*model.Connections, error)
	GetStats() (map[string]interface{}, error)
	GetProcStats(pids []int32) (*model.ProcStatsWithPermByPID, error)
	Register(clientID string) error
}
