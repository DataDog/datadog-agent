package encoding

import (
	"sync"

	model "github.com/DataDog/agent-payload/process"
)

var statPool = sync.Pool{
	New: func() interface{} {
		return new(model.ProcStatsWithPerm)
	},
}

var statsPool = sync.Pool{
	New: func() interface{} {
		s := new(model.ProcStatsWithPermByPID)
		s.StatsByPID = make(map[int32]*model.ProcStatsWithPerm)
		return s
	},
}

func returnToPool(stats *model.ProcStatsWithPermByPID) {
	if stats.StatsByPID != nil {
		for _, s := range stats.StatsByPID {
			statPool.Put(s)
		}
	}

	statsPool.Put(stats)
}
