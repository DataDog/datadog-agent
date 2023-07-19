package process

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	workloadmetaExtractor "github.com/DataDog/datadog-agent/pkg/process/metadata/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const collectorId = "process"

const collectionInterval = 1 * time.Minute

func init() {
	workloadmeta.RegisterProcessAgentCollector(collectorId, func() workloadmeta.Collector {
		// TODO: Inject config.Datadog via fx once collectors are migrated to components.
		ddConfig := config.Datadog

		wlmExtractor := workloadmetaExtractor.NewWorkloadMetaExtractor(ddConfig)

		processData := checks.NewProcessData(ddConfig)
		processData.Register(wlmExtractor)

		return &collector{
			ddConfig:     ddConfig,
			wlmExtractor: wlmExtractor,
			grpcServer:   workloadmetaExtractor.NewGRPCServer(ddConfig, wlmExtractor),
			processData:  processData,
			pidToCid:     make(map[int]string),
		}
	})
}

var _ workloadmeta.Collector = (*collector)(nil)

type collector struct {
	ddConfig config.ConfigReader

	processData *checks.ProcessData

	wlmExtractor *workloadmetaExtractor.WorkloadMetaExtractor
	grpcServer   *workloadmetaExtractor.GRPCServer

	pidToCid map[int]string

	store workloadmeta.Store
}

func (c *collector) Start(ctx context.Context, store workloadmeta.Store) error {
	c.store = store

	collectionTicker := time.NewTicker(collectionInterval)
	defer collectionTicker.Stop()

	filter := workloadmeta.NewFilter([]workloadmeta.Kind{workloadmeta.KindContainer}, workloadmeta.SourceAll, workloadmeta.EventTypeAll)
	containerEvt := store.Subscribe("process_collector", workloadmeta.NormalPriority, filter)
	go func() {
		for {
			select {
			case evt := <-containerEvt:
				c.handleContainerEvent(evt)
			case <-collectionTicker.C:
				err := c.processData.Fetch()
				_ = log.Error("Error fetching process data:", err)
			case <-ctx.Done():
				c.grpcServer.Stop()
				return
			}
		}
	}()
	return nil
}

// Pull is unused at the moment used due to the short frequency in which it is called.
// In the future, we should use it to locally in workload-meta.
func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) handleContainerEvent(evt workloadmeta.EventBundle) {
	defer close(evt.Ch)

	for _, evt := range evt.Events {
		ent := evt.Entity.(*workloadmeta.Container)
		switch evt.Type {
		case workloadmeta.EventTypeSet:
			// Should be safe, even on windows because PID 0 is the idle process and therefore must always belong to the host
			if ent.PID != 0 {
				c.pidToCid[ent.PID] = ent.ID
			}
		case workloadmeta.EventTypeUnset:
			delete(c.pidToCid, ent.PID)
		}
	}

	c.wlmExtractor.SetLastPidToCid(c.pidToCid)
}
