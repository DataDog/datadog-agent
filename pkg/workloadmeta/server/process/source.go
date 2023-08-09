package process

import "github.com/DataDog/datadog-agent/pkg/languagedetection/languagemodels"

type Source interface {
	ListProcesses() (map[string]*Entity, int32)
	Subscribe() <-chan *CacheDiff
}

// CacheDiff holds the information about processes that have been created and deleted in the past
// Extract call from the WorkloadMetaExtractor cache
type CacheDiff struct {
	CacheVersion int32
	Creation     []*Entity
	Deletion     []*Entity
}

// Entity represents a process exposed by the WorkloadMeta extractor
type Entity struct {
	Pid          int32
	ContainerId  string
	NsPid        int32
	CreationTime int64
	Language     *languagemodels.Language
}
