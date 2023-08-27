package workloadmeta

import "github.com/DataDog/datadog-agent/pkg/workloadmeta"

type ProcessSource interface {
	GetAllProcessEntities() (map[string]*workloadmeta.Process, int32)
	ProcessCacheDiff() <-chan *ProcessCacheDiff
}
