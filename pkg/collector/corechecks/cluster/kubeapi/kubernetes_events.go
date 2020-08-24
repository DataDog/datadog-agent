// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package kubeapi

import (
	"fmt"
	"strings"
	"time"

	cache "github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"
	"k8s.io/api/core/v1"

	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/clustername"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// Covers the Control Plane service check and the in memory pod metadata.
const (
	kubernetesAPIEventsCheckName  = "kubernetes_api_events"
	eventTokenKey                 = "event"
	maxEventCardinality           = 300
	defaultResyncPeriodInSecond   = 300
	defaultTimeoutEventCollection = 2000

	defaultCacheExpire = 2 * time.Minute
	defaultCachePurge  = 10 * time.Minute
)

// KubeApiEventsConfig is the config of the API server.
type EventsConfig struct {
	CollectEvent             bool     `yaml:"collect_events"`
	FilteredEventTypes       []string `yaml:"filtered_event_types"`
	EventCollectionTimeoutMs int      `yaml:"kubernetes_event_read_timeout_ms"`
	MaxEventCollection       int      `yaml:"max_events_per_run"`
	ResyncPeriodEvents       int      `yaml:"kubernetes_event_resync_period_s"`
}

// EventC holds the information pertaining to which event we collected last and when we last re-synced.
type EventC struct {
	LastResVer string
	LastTime   time.Time
}

// KubeApiEventsCheck grabs events from the API server.
type EventsCheck struct {
	CommonCheck
	instance        *EventsConfig
	eventCollection EventC
	ignoredEvents   string
	providerIDCache *cache.Cache
}

func (c *EventsConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = config.Datadog.GetBool("collect_kubernetes_events")
	c.ResyncPeriodEvents = defaultResyncPeriodInSecond

	return yaml.Unmarshal(data, c)
}

// NewKubernetesAPIEventsCheck creates a instance of the kubernetes EventsCheck given the base and instance
func NewKubernetesAPIEventsCheck(base core.CheckBase, instance *EventsConfig) *EventsCheck {
	return &EventsCheck{
		CommonCheck: CommonCheck{
			CheckBase: base,
		},
		instance:        instance,
		providerIDCache: cache.New(defaultCacheExpire, defaultCachePurge),
	}
}

// KubernetesAPIEventsFactory is exported for integration testing.
func KubernetesAPIEventsFactory() check.Check {
	return NewKubernetesAPIEventsCheck(core.NewCheckBase(kubernetesAPIEventsCheckName), &EventsConfig{})
}

// Configure parses the check configuration and init the check.
func (k *EventsCheck) Configure(config, initConfig integration.Data, source string) error {
	err := k.ConfigureKubeAPICheck(config, source)
	if err != nil {
		return err
	}

	// Check connectivity to the APIServer
	err = k.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the API events check")
		return err
	}

	if k.instance.EventCollectionTimeoutMs == 0 {
		k.instance.EventCollectionTimeoutMs = defaultTimeoutEventCollection
	}

	if k.instance.MaxEventCollection == 0 {
		k.instance.MaxEventCollection = maxEventCardinality
	}

	k.ignoredEvents = convertFilter(k.instance.FilteredEventTypes)

	log.Debugf("Running config %s", config)
	return nil
}

func convertFilter(conf []string) string {
	var formatedFilters []string
	for _, filter := range conf {
		f := strings.Split(filter, "=")
		if len(f) == 1 {
			formatedFilters = append(formatedFilters, fmt.Sprintf("reason!=%s", f[0]))
			continue
		}
		formatedFilters = append(formatedFilters, filter)
	}
	return strings.Join(formatedFilters, ",")
}

// Run executes the check.
func (k *EventsCheck) Run() error {
	// initialize kube api check
	err := k.InitKubeAPICheck()
	if err == apiserver.ErrNotLeader {
		log.Debug("Agent is not leader, will not run the check")
		return nil
	} else if err != nil {
		return err
	}

	// Running the event collection.
	if !k.instance.CollectEvent {
		return nil
	}

	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}
	defer sender.Commit()

	// Get the events from the API server
	events, err := k.eventCollectionCheck()
	if err != nil {
		return err
	}

	// Process the events to have a Datadog format.
	err = k.processEvents(sender, events)
	if err != nil {
		_ = k.Warnf("Could not submit new event %s", err.Error())
	}
	return nil
}

func (k *EventsCheck) eventCollectionCheck() (newEvents []*v1.Event, err error) {
	resVer, lastTime, err := k.ac.GetTokenFromConfigmap(eventTokenKey)
	if err != nil {
		return nil, err
	}

	// This is to avoid getting in a situation where we list all the events for multiple runs in a row.
	if resVer == "" && k.eventCollection.LastResVer != "" {
		log.Errorf("Resource Version stored in the ConfigMap is incorrect. Will resume collecting from %s", k.eventCollection.LastResVer)
		resVer = k.eventCollection.LastResVer
	}

	timeout := int64(k.instance.EventCollectionTimeoutMs / 1000)
	limit := int64(k.instance.MaxEventCollection)
	resync := int64(k.instance.ResyncPeriodEvents)
	newEvents, k.eventCollection.LastResVer, k.eventCollection.LastTime, err = k.ac.RunEventCollection(resVer, lastTime, timeout, limit, resync, k.ignoredEvents)

	if err != nil {
		k.Warnf("Could not collect events from the api server: %s", err.Error()) //nolint:errcheck
		return nil, err
	}

	configMapErr := k.ac.UpdateTokenInConfigmap(eventTokenKey, k.eventCollection.LastResVer, k.eventCollection.LastTime)
	if configMapErr != nil {
		k.Warnf("Could not store the LastEventToken in the ConfigMap: %s", configMapErr.Error()) //nolint:errcheck
	}
	return newEvents, nil
}

// processEvents:
// - iterates over the Kubernetes Events
// - extracts some attributes and builds a structure ready to be submitted as a Datadog event (bundle)
// - formats the bundle and submit the Datadog event
func (k *EventsCheck) processEvents(sender aggregator.Sender, events []*v1.Event) error {
	eventsByObject := make(map[string]*kubernetesEventBundle)

	for _, event := range events {
		id := bundleID(event)
		bundle, found := eventsByObject[id]
		if found == false {
			bundle = newKubernetesEventBundler(event)
			eventsByObject[id] = bundle
		}
		err := bundle.addEvent(event)
		if err != nil {
			k.Warnf("Error while bundling events, %s.", err.Error()) //nolint:errcheck
		}
	}
	clusterName := clustername.GetClusterName()
	for _, bundle := range eventsByObject {
		datadogEv, err := bundle.formatEvents(clusterName, k.providerIDCache)
		if err != nil {
			k.Warnf("Error while formatting bundled events, %s. Not submitting", err.Error()) //nolint:errcheck
			continue
		}
		sender.Event(datadogEv)
	}
	return nil
}

// bundleID generates a unique ID to separate k8s events
// based on their InvolvedObject UIDs and event Types
func bundleID(e *v1.Event) string {
	return fmt.Sprintf("%s/%s", e.InvolvedObject.UID, e.Type)
}

func init() {
	core.RegisterCheck(kubernetesAPIEventsCheckName, KubernetesAPIEventsFactory)
}
