// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const tagCompletenessRetryInterval = 1 * time.Second

// workloadmetaListener is a generic subscriber to workloadmeta events that
// generates AD services.
type workloadmetaListener interface {
	ServiceListener

	// Store returns a reference to the workloadmeta store being used by
	// the listener.
	Store() workloadmeta.Component

	// AddService creates a new AD service under the svcID name (only used
	// internally to identify a service). If a non-empty parentSvcID is
	// passed, the service will be deleted when the parent service is
	// removed.
	AddService(svcID string, svc Service, parentSvcID string)
}

type pendingEntityInfo struct {
	entity    workloadmeta.Entity
	firstSeen time.Time
}

// workloadmetaListenerImpl implements workloadmetaListener.
type workloadmetaListenerImpl struct {
	name string
	stop chan struct{}

	processFn func(workloadmeta.Entity)

	store           workloadmeta.Component
	workloadFilters *workloadmeta.Filter

	services map[string]Service
	children map[string]map[string]struct{}

	newService chan<- Service
	delService chan<- Service

	telemetryStore *telemetry.Store

	isReadyFn       func(workloadmeta.Entity) bool // when nil, consider ready
	pendingEntities map[string]pendingEntityInfo   // svcID → pending info
	maxWaitDuration time.Duration
}

var _ workloadmetaListener = &workloadmetaListenerImpl{}

// newWorkloadmetaListener returns a new workloadmetaListener. It filters
// workloadmeta events with the passed in workloadFilters, and processes each
// event with processFn. processFn is expected to create AD services by calling
// AddService. Services are removed automatically on
// workloadmeta.EventTypeUnset events, including child services when the parent
// service is removed.
func newWorkloadmetaListener(
	name string,
	workloadFilters *workloadmeta.Filter,
	processFn func(workloadmeta.Entity),
	wmeta workloadmeta.Component,
	telemetryStore *telemetry.Store,
) (workloadmetaListener, error) {
	return &workloadmetaListenerImpl{
		name: name,
		stop: make(chan struct{}),

		processFn:       processFn,
		store:           wmeta,
		workloadFilters: workloadFilters,

		services: make(map[string]Service),
		children: make(map[string]map[string]struct{}),

		telemetryStore: telemetryStore,
	}, nil
}

// newWorkloadmetaListenerWithTagWait is like newWorkloadmetaListener but defers
// processing until isReadyFn returns true. Disabled when maxWait is 0.
func newWorkloadmetaListenerWithTagWait(
	name string,
	workloadFilters *workloadmeta.Filter,
	processFn func(workloadmeta.Entity),
	wmeta workloadmeta.Component,
	telemetryStore *telemetry.Store,
	isReadyFn func(workloadmeta.Entity) bool,
	maxWait time.Duration,
) (workloadmetaListener, error) {
	base, err := newWorkloadmetaListener(name, workloadFilters, processFn, wmeta, telemetryStore)
	if err != nil {
		return nil, err
	}

	if maxWait > 0 {
		listener, ok := base.(*workloadmetaListenerImpl)
		if !ok {
			return nil, fmt.Errorf("unexpected listener type %T", base)
		}

		listener.isReadyFn = isReadyFn
		listener.pendingEntities = make(map[string]pendingEntityInfo)
		listener.maxWaitDuration = maxWait
	}

	return base, nil
}

func (l *workloadmetaListenerImpl) Store() workloadmeta.Component {
	return l.store
}

func (l *workloadmetaListenerImpl) AddService(svcID string, svc Service, parentSvcID string) {
	kind := kindFromSvcID(svcID)
	if parentSvcID != "" {
		if _, ok := l.children[parentSvcID]; !ok {
			l.children[parentSvcID] = make(map[string]struct{})
		}

		l.children[parentSvcID][svcID] = struct{}{}
	}

	if old, found := l.services[svcID]; found {
		if svc.Equal(old) {
			log.Tracef("%s received a duplicated service '%s', ignoring", l.name, svc.GetServiceID())
			return
		}

		log.Tracef("%s received an updated service '%s', removing the old one", l.name, svc.GetServiceID())
		l.delService <- old
		if l.telemetryStore != nil {
			l.telemetryStore.WatchedResources.Dec(l.name, kind)
		}
	}

	l.services[svcID] = svc
	l.newService <- svc
	if l.telemetryStore != nil {
		l.telemetryStore.WatchedResources.Inc(l.name, kind)
	}
}

func (l *workloadmetaListenerImpl) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	l.newService = newSvc
	l.delService = delSvc

	ch := l.store.Subscribe(l.name, workloadmeta.NormalPriority, l.workloadFilters)
	health := health.RegisterLiveness(l.name)

	log.Infof("%s initialized successfully", l.name)

	go func() {
		var retryTicker *time.Ticker
		var retryChan <-chan time.Time
		if l.isReadyFn != nil {
			retryTicker = time.NewTicker(tagCompletenessRetryInterval)
			retryChan = retryTicker.C
		}

		defer func() {
			if retryTicker != nil {
				retryTicker.Stop()
			}
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
		}()

		for {
			select {
			case evBundle, ok := <-ch:
				if !ok {
					return
				}

				l.processEvents(evBundle)

			case <-retryChan:
				l.retryPendingEntities()

			case <-health.C:

			case <-l.stop:
				l.store.Unsubscribe(ch)

				return
			}
		}
	}()
}

func (l *workloadmetaListenerImpl) Stop() {
	close(l.stop)
}

func (l *workloadmetaListenerImpl) processEvents(evBundle workloadmeta.EventBundle) {
	// Acknowledge the bundle since there are no downstream
	// collectors that depend on AD having up to date data.
	evBundle.Acknowledge()

	for _, ev := range evBundle.Events {
		entity := ev.Entity

		switch ev.Type {
		case workloadmeta.EventTypeSet:
			l.processSetEntity(entity)

		case workloadmeta.EventTypeUnset:
			l.processUnsetEntity(entity)

		default:
			log.Errorf("cannot handle event of type %d", ev.Type)
		}
	}
}

func (l *workloadmetaListenerImpl) processSetEntity(entity workloadmeta.Entity) {
	svcID := buildSvcID(entity.GetID())

	if l.isReadyFn != nil && l.waitIfNotReady(svcID, entity) {
		return
	}

	// keep track of children of this entity from previous iterations ...
	unseen := make(map[string]struct{})
	for childSvcID := range l.children[svcID] {
		unseen[childSvcID] = struct{}{}
	}

	// ... and create a new empty map to store the children seen in this
	// iteration.
	l.children[svcID] = make(map[string]struct{})

	l.processFn(entity)

	// remove the children seen in this iteration from the unseen list ...
	for childSvcID := range l.children[svcID] {
		delete(unseen, childSvcID)
	}

	// ... and remove services for everything that has been left
	for childSvcID := range unseen {
		l.removeService(childSvcID)
	}
}

// waitIfNotReady returns true if the entity is not ready and should be retried
// later.
func (l *workloadmetaListenerImpl) waitIfNotReady(svcID string, entity workloadmeta.Entity) bool {
	if l.isReadyFn(entity) {
		_, wasPending := l.pendingEntities[svcID]
		l.resolvePending(svcID)
		if !wasPending && l.telemetryStore != nil {
			l.telemetryStore.TagCompletenessDelay.Observe(0, l.name)
		}
		return false
	}

	now := time.Now()

	pending, exists := l.pendingEntities[svcID]
	if !exists {
		pending = pendingEntityInfo{
			entity:    entity,
			firstSeen: now,
		}
	} else {
		pending.entity = entity
	}

	if now.Sub(pending.firstSeen) < l.maxWaitDuration {
		l.pendingEntities[svcID] = pending
		log.Debugf("%s not adding entity %s: tags not complete yet", l.name, svcID)
		return true
	}

	// Timeout exceeded
	log.Warnf("%s adding entity %s with potentially incomplete tags", l.name, svcID)
	l.resolvePending(svcID)

	return false
}

func (l *workloadmetaListenerImpl) resolvePending(svcID string) {
	pending, wasPending := l.pendingEntities[svcID]
	if !wasPending {
		return
	}

	delay := time.Since(pending.firstSeen).Seconds()
	if l.telemetryStore != nil {
		l.telemetryStore.TagCompletenessDelay.Observe(delay, l.name)
	}
	delete(l.pendingEntities, svcID)
}

func (l *workloadmetaListenerImpl) retryPendingEntities() {
	pending := make([]workloadmeta.Entity, 0, len(l.pendingEntities))
	for _, pendingEntity := range l.pendingEntities {
		pending = append(pending, pendingEntity.entity)
	}

	for _, entity := range pending {
		l.processSetEntity(entity)
	}
}

func (l *workloadmetaListenerImpl) processUnsetEntity(entity workloadmeta.Entity) {
	entityID := entity.GetID()
	parentSvcID := buildSvcID(entityID)

	if l.pendingEntities != nil {
		delete(l.pendingEntities, parentSvcID)
	}

	l.removeService(parentSvcID)

	childrenSvcIDs := l.children[parentSvcID]
	delete(l.children, parentSvcID)

	for svcID := range childrenSvcIDs {
		l.removeService(svcID)
	}
}

func (l *workloadmetaListenerImpl) removeService(svcID string) {
	svc, ok := l.services[svcID]
	if !ok {
		log.Debugf("service %q not found, not removing", svcID)
		return
	}

	delete(l.services, svcID)
	l.delService <- svc
	if l.telemetryStore != nil {
		l.telemetryStore.WatchedResources.Dec(l.name, kindFromSvcID(svcID))
	}
}

func buildSvcID(entityID workloadmeta.EntityID) string {
	return fmt.Sprintf("%s://%s", entityID.Kind, entityID.ID)
}

func kindFromSvcID(svcID string) string {
	sep := "://"
	if strings.Contains(svcID, sep) {
		return strings.Split(svcID, sep)[0]
	}

	return "unknown"
}
