// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package queryactionsimpl implements the Data Observability query actions component
package queryactionsimpl

import (
	"context"
	"sync"
	"time"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	doqueryactions "github.com/DataDog/datadog-agent/comp/dataobs/queryactions/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"go.yaml.in/yaml/v3"
)

// Requires defines the dependencies for the Data Observability query actions component
type Requires struct {
	Lc       compdef.Lifecycle
	Log      log.Component
	RcClient rcclient.Component
	Ac       autodiscovery.Component
	// EventPlatform sends task-level error results for one-off tasks the agent cannot start.
	EventPlatform eventplatform.Component
	// Collector reports when a one-off task's check has run, so the next task queued for the same
	// database can start right away. Without it, a queued task starts when the previous task's
	// config leaves the RC snapshot.
	Collector option.Option[collector.Component]
}

// Provides defines the output of the Data Observability query actions component
type Provides struct {
	Comp doqueryactions.Component
}

// component implements the Data Observability query actions component
type component struct {
	log      log.Component
	ac       autodiscovery.Component
	rcclient rcclient.Component
	// activeConfigs maps a DO config_id to the DO check config currently scheduled for it.
	activeConfigs map[string]activeConfigEntry
	// managedBases maps a base integration config Digest to the bookkeeping needed to restore it.
	// A base config has an entry here while at least one DO config targets one of its instances;
	// the entry records the original config (for restoration) and the remainder config currently
	// scheduled in its place. See reconcileBases.
	managedBases    map[string]*managedBaseEntry
	activeConfigsMu sync.Mutex

	// tasks maps the RC config ID of each one-off task in the latest snapshot to its outcome. See
	// tasks.go.
	tasks   map[string]*trackedTask
	tasksMu sync.Mutex
	// taskChanges carries task check configs to the task provider, which streams them to
	// autodiscovery separately from monitor configs.
	taskChanges   *taskChangesQueue
	eventPlatform eventplatform.Component
	now           func() time.Time
	// taskApplyStatus is the apply state callback of the latest RC update, used to report the state
	// of a queued task that starts between RC updates.
	taskApplyStatus func(string, state.ApplyStatus)
	// finishedTaskChecks returns the config IDs of task checks that have completed their run. Nil
	// when there is no collector.
	finishedTaskChecks func() map[string]bool
	stopTaskPolling    context.CancelFunc
}

// NewComponent creates a new Data Observability query actions component
func NewComponent(reqs Requires) (Provides, error) {
	c := &component{
		log:           reqs.Log,
		ac:            reqs.Ac,
		rcclient:      reqs.RcClient,
		activeConfigs: make(map[string]activeConfigEntry),
		managedBases:  make(map[string]*managedBaseEntry),
		tasks:         make(map[string]*trackedTask),
		taskChanges:   newTaskChangesQueue(),
		eventPlatform: reqs.EventPlatform,
		now:           time.Now,
	}
	if coll, ok := reqs.Collector.Get(); ok {
		c.finishedTaskChecks = collectorFinishedTaskChecks(coll)
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})

	return Provides{Comp: c}, nil
}

func (c *component) start(_ context.Context) error {
	c.ac.AddConfigProvider(c, false, 0)
	c.ac.AddConfigProvider(&taskProvider{queue: c.taskChanges}, false, 0)
	if c.finishedTaskChecks != nil {
		ctx, cancel := context.WithCancel(context.Background())
		c.stopTaskPolling = cancel
		go c.pollTaskChecks(ctx, taskCheckPollInterval)
	}
	c.log.Info("Data Observability query actions component started")
	return nil
}

func (c *component) stop(_ context.Context) error {
	if c.stopTaskPolling != nil {
		c.stopTaskPolling()
	}
	return nil
}

// String returns the name of the provider
func (c *component) String() string {
	return names.DOQueryActions
}

// GetConfigErrors is required by the ConfigProvider interface. This provider does not track
// per-resource errors; RC apply errors are reported via the applyStatus callback in onRCUpdate.
func (c *component) GetConfigErrors() map[string]types.ErrorMsgSet {
	return map[string]types.ErrorMsgSet{}
}

// Stream creates a fresh channel per call (same pattern as container/process_log providers).
// An empty ConfigChanges is sent immediately because autodiscovery's LoadAndRun iterates
// providers sequentially and blocks on each streaming provider until its first message arrives,
// before proceeding to the next provider.
//
// The RC callback writes to outCh with merge semantics: if autodiscovery hasn't consumed the
// previous update yet, it is merged into the latest one (see mergeConfigChanges). onRCUpdate only
// emits what changed, so a dropped update's entries are never sent again and must be kept.
// outCh is closed when ctx is cancelled so the config poller goroutine can observe teardown.
func (c *component) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	outCh := make(chan integration.ConfigChanges, 1)
	// Unblock autodiscovery's LoadAndRun — it blocks on <-ch until the first message arrives.
	outCh <- integration.ConfigChanges{}

	var (
		mu     sync.Mutex
		closed bool
	)

	// sendChanges delivers changes to outCh under mu. The channel is capacity-1; when full,
	// the old entry is drained and merged into changes. mu also guards against writing to a
	// closed channel after shutdown.
	sendChanges := func(changes integration.ConfigChanges) {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		select {
		case outCh <- changes:
		default:
			var dropped integration.ConfigChanges
			select {
			case dropped = <-outCh:
			default:
			}
			outCh <- mergeConfigChanges(dropped, changes) // safe: mu held, closed=false, channel was just drained
		}
	}

	// subscribeAndWait subscribes to the RC product and blocks until ctx is cancelled.
	subscribeAndWait := func() {
		c.rcclient.Subscribe(data.ProductDOQueryActions, func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
			changes := c.onRCUpdate(updates, applyStatus)
			if !changes.IsEmpty() {
				sendChanges(changes)
			}
		})
		c.log.Info("Subscribed to RC DO_QUERY_ACTIONS product for Data Observability query actions")
		<-ctx.Done()
	}

	go func() {
		defer func() {
			// Close outCh so the config poller goroutine can observe shutdown.
			// mu prevents a concurrent RC callback from writing to the closed channel.
			mu.Lock()
			defer mu.Unlock()
			closed = true
			close(outCh)
		}()

		// Check immediately: the file config provider runs before this one in LoadAndRun,
		// so a supported integration is typically already available when Stream() is called.
		if c.hasSupportedIntegration() {
			subscribeAndWait()
			return
		}

		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if c.hasSupportedIntegration() {
					subscribeAndWait()
					return
				}
			}
		}
	}()

	return outCh
}

// hasSupportedIntegration checks if any supported DB integration with
// data_observability.enabled: true is configured in autodiscovery.
func (c *component) hasSupportedIntegration() bool {
	for _, cfg := range c.ac.GetUnresolvedConfigs() {
		if !isSupportedIntegration(cfg.Name) {
			continue
		}
		for _, instanceData := range cfg.Instances {
			var instance map[string]any
			if err := yaml.Unmarshal(instanceData, &instance); err != nil {
				continue
			}
			if instanceHasDOEnabled(instance) {
				return true
			}
		}
	}
	return false
}

// mergeConfigChanges combines an undelivered update with the one that follows it, as if both had
// been applied in order. Autodiscovery applies every Unschedule before any Schedule, so:
//   - every Unschedule of both updates is kept, so no check already running is orphaned;
//   - a Schedule of the older update is kept unless the newer one unschedules the same config,
//     which means the newer snapshot removed or replaced it. Checks left unchanged by the newer
//     update appear in neither of its lists, so their older Schedule must survive.
func mergeConfigChanges(older, newer integration.ConfigChanges) integration.ConfigChanges {
	unscheduled := make(map[string]bool, len(newer.Unschedule))
	for _, cfg := range newer.Unschedule {
		unscheduled[cfg.Digest()] = true
	}
	scheduled := make(map[string]bool, len(older.Schedule)+len(newer.Schedule))
	merged := integration.ConfigChanges{
		Unschedule: append(append([]integration.Config(nil), older.Unschedule...), newer.Unschedule...),
	}
	for _, cfg := range older.Schedule {
		digest := cfg.Digest()
		if unscheduled[digest] || scheduled[digest] {
			continue
		}
		scheduled[digest] = true
		merged.Schedule = append(merged.Schedule, cfg)
	}
	for _, cfg := range newer.Schedule {
		digest := cfg.Digest()
		if scheduled[digest] {
			continue
		}
		scheduled[digest] = true
		merged.Schedule = append(merged.Schedule, cfg)
	}
	return merged
}
