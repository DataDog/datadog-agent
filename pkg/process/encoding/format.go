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

func returnToPool(stats map[int32]*model.ProcStatsWithPerm) {
	for _, s := range stats {
		statPool.Put(s)
	}
}
