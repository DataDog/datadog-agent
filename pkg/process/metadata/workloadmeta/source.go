package workloadmeta

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"sync"
)

type ProcessSource interface {
	GetAllProcessEntities() (map[string]*workloadmeta.Process, int32)
	ProcessCacheDiff() <-chan *ProcessCacheDiff
}

type WorkloadMetaSource struct {
	mutex        *sync.Mutex
	diff         chan *ProcessCacheDiff
	store        workloadmeta.Store
	cacheVersion int32
	evts         chan workloadmeta.EventBundle
}

func NewWorkloadMetaSource(store workloadmeta.Store) *WorkloadMetaSource {
	return &WorkloadMetaSource{
		mutex: &sync.Mutex{},
		diff:  make(chan *ProcessCacheDiff, 1),
		store: store,
	}
}

func (w *WorkloadMetaSource) Start() {
	w.evts = w.store.Subscribe("WorkloadMetaSource", workloadmeta.NormalPriority, workloadmeta.NewFilter([]workloadmeta.Kind{
		workloadmeta.KindProcess,
	}, workloadmeta.SourceAll, workloadmeta.EventTypeAll))
	go func() {
		for evt := range w.evts {
			func() {
				w.mutex.Lock()
				defer w.mutex.Unlock()

				w.cacheVersion++
				w.diff <- eventBundleToProcessEvent(evt, w.cacheVersion)
			}()
		}
	}()
}

func (w *WorkloadMetaSource) Stop() {
	w.store.Unsubscribe(w.evts)
}

func (w *WorkloadMetaSource) GetAllProcessEntities() (map[string]*workloadmeta.Process, int32) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	procsInStore := w.store.ListProcesses()
	result := make(map[string]*workloadmeta.Process, len(procsInStore))
	for _, proc := range procsInStore {
		result[proc.ID] = proc
	}
	return result, w.cacheVersion
}

func (w *WorkloadMetaSource) ProcessCacheDiff() <-chan *ProcessCacheDiff {
	return w.diff
}

func eventBundleToProcessEvent(bundle workloadmeta.EventBundle, version int32) *ProcessCacheDiff {
	defer close(bundle.Ch)

	diff := &ProcessCacheDiff{
		cacheVersion: version,
		creation:     make([]*workloadmeta.Process, 0, len(bundle.Events)),
		deletion:     make([]*workloadmeta.Process, 0, len(bundle.Events)),
	}
	for _, evt := range bundle.Events {
		entity, ok := evt.Entity.(*workloadmeta.Process)
		if !ok {
			log.Warnf("unexpected entity type: %T", evt.Entity)
			continue
		}

		switch evt.Type {
		case workloadmeta.EventTypeSet:
			diff.creation = append(diff.creation, entity)
		case workloadmeta.EventTypeUnset:
			diff.deletion = append(diff.deletion, entity)
		}
	}

	return diff
}
