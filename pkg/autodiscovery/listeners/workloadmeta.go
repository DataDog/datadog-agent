// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build !serverless

package listeners

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/telemetry"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// workloadmetaListener is a generic subscriber to workloadmeta events that
// generates AD services.
type workloadmetaListener interface {
	ServiceListener

	// Store returns a reference to the workloadmeta store being used by
	// the listener.
	Store() workloadmeta.Store

	// AddService creates a new AD service under the svcID name (only used
	// internally to identify a service). If a non-empty parentSvcID is
	// passed, the service will be deleted when the parent service is
	// removed.
	AddService(svcID string, svc Service, parentSvcID string)

	// IsExcluded returns whether a container should be excluded according
	// to the chosen ft filter.
	IsExcluded(ft containers.FilterType, annotations map[string]string, name, image, ns string) bool
}

// workloadmetaListenerImpl implements workloadmetaListener.
type workloadmetaListenerImpl struct {
	name string
	stop chan struct{}

	processFn func(workloadmeta.Entity)

	store            workloadmeta.Store
	workloadFilters  *workloadmeta.Filter
	containerFilters *containerFilters

	services map[string]Service
	children map[string]map[string]struct{}

	newService chan<- Service
	delService chan<- Service
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
) (workloadmetaListener, error) {
	containerFilters, err := newContainerFilters()
	if err != nil {
		return nil, err
	}

	return &workloadmetaListenerImpl{
		name: name,
		stop: make(chan struct{}),

		processFn: processFn,

		store:            workloadmeta.GetGlobalStore(),
		workloadFilters:  workloadFilters,
		containerFilters: containerFilters,

		services: make(map[string]Service),
		children: make(map[string]map[string]struct{}),
	}, nil
}

func (l *workloadmetaListenerImpl) Store() workloadmeta.Store {
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
		if svcEqual(old, svc) {
			log.Tracef("%s received a duplicated service '%s', ignoring", l.name, svc.GetServiceID())
			return
		}

		log.Tracef("%s received an updated service '%s', removing the old one", l.name, svc.GetServiceID())
		l.delService <- old
		telemetry.WatchedResources.Dec(l.name, kind)
	}

	l.services[svcID] = svc
	l.newService <- svc
	telemetry.WatchedResources.Inc(l.name, kind)
}

func (l *workloadmetaListenerImpl) IsExcluded(ft containers.FilterType, annotations map[string]string, name, image, ns string) bool {
	return l.containerFilters.IsExcluded(ft, annotations, name, image, ns)
}

func (l *workloadmetaListenerImpl) Listen(newSvc chan<- Service, delSvc chan<- Service) {
	l.newService = newSvc
	l.delService = delSvc

	ch := l.store.Subscribe(l.name, workloadmeta.NormalPriority, l.workloadFilters)
	health := health.RegisterLiveness(l.name)

	log.Infof("%s initialized successfully", l.name)

	go func() {
		defer func() {
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
	// close the bundle channel asap since there are no downstream
	// collectors that depend on AD having up to date data.
	close(evBundle.Ch)

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

func (l *workloadmetaListenerImpl) processUnsetEntity(entity workloadmeta.Entity) {
	entityID := entity.GetID()
	parentSvcID := buildSvcID(entityID)

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
	telemetry.WatchedResources.Dec(l.name, kindFromSvcID(svcID))
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
