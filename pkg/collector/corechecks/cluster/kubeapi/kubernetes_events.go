// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package kubeapi

import (
	"fmt"
	"strings"
	"time"

	cache "github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"

	"github.com/StackVista/stackstate-agent/pkg/aggregator"
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	"github.com/StackVista/stackstate-agent/pkg/config"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver/leaderelection"
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

// EventsConfig is the config of the API server.
type EventsConfig struct {
	CollectEvent             bool     `yaml:"collect_events"`
	FilteredEventTypes       []string `yaml:"filtered_event_types"`
	EventCollectionTimeoutMs int      `yaml:"kubernetes_event_read_timeout_ms"`
	MaxEventCollection       int      `yaml:"max_events_per_run"`
	LeaderSkip               bool     `yaml:"skip_leader_election"`
	ResyncPeriodEvents       int      `yaml:"kubernetes_event_resync_period_s"`
}

// EventC holds the information pertaining to which event we collected last and when we last re-synced.
type EventC struct {
	LastResVer string
	LastTime   time.Time
}

// EventsCheck grabs events from the API server.
type EventsCheck struct {
	CommonCheck
	instance        *EventsConfig
	eventCollection EventC
	ignoredEvents   string
	providerIDCache *cache.Cache
	mapperFactory   KubernetesEventMapperFactory
}

func (c *EventsConfig) parse(data []byte) error {
	// default values
	c.CollectEvent = config.Datadog.GetBool("collect_kubernetes_events")
	c.ResyncPeriodEvents = defaultResyncPeriodInSecond
	c.LeaderSkip = true

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
		mapperFactory:   newKubernetesEventMapper,
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
	// Running the event collection.
	if !k.instance.CollectEvent {
		return nil
	}

	sender, err := aggregator.GetSender(k.ID())
	if err != nil {
		return err
	}
	defer sender.Commit()

	// If the check is configured as a cluster check, the cluster check worker needs to skip the leader election section.
	// The Cluster Agent will passed in the `skip_leader_election` bool.
	if !k.instance.LeaderSkip {
		// Only run if Leader Election is enabled.
		if !config.Datadog.GetBool("leader_election") {
			return log.Error("Leader Election not enabled. Not running Kubernetes API Server check or collecting Kubernetes Events.")
		}
		errLeader := k.runLeaderElection()
		if errLeader != nil {
			if errLeader == apiserver.ErrNotLeader {
				// Only the leader can instantiate the apiserver client.
				log.Debug("Agent is not leader, will not run the check")
				return nil
			}
			return err
		}
	}

	// initialize kube api check
	err = k.InitKubeAPICheck()
	if err != nil {
		return err
	}

	// Running the event collection.
	if !k.instance.CollectEvent {
		return nil
	}

	// Get the events from the API server
	events, err := k.eventCollectionCheck()
	if err != nil {
		return err
	}

	// Process the events to have a Datadog format.
	err = k.processEvents(sender, events)
	if err != nil {
		k.Warnf("Could not submit new event %s", err.Error()) //nolint:errcheck
	}
	return nil
}

func (k *EventsCheck) runLeaderElection() error {

	leaderEngine, err := leaderelection.GetLeaderEngine()
	if err != nil {
		k.Warn("Failed to instantiate the Leader Elector. Not running the Kubernetes API Server check or collecting Kubernetes Events.") //nolint:errcheck
		return err
	}

	err = leaderEngine.EnsureLeaderElectionRuns()
	if err != nil {
		k.Warn("Leader Election process failed to start") //nolint:errcheck
		return err
	}

	if !leaderEngine.IsLeader() {
		log.Debugf("Leader is %q. %s will not run Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader(), leaderEngine.HolderIdentity)
		return apiserver.ErrNotLeader
	}
	log.Tracef("Current leader: %q, running Kubernetes cluster related checks and collecting events", leaderEngine.GetLeader())
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
// - extracts some attributes and builds a structure ready to be submitted as a StackState event
// - convert each K8s event to a metrics event to be processed by the intake
func (k *EventsCheck) processEvents(sender aggregator.Sender, events []*v1.Event) error {
	clusterName := clustername.GetClusterName()
	mapper := k.mapperFactory(k.ac, clusterName)
	for _, event := range events {
		mappedEvent, err := mapper.mapKubernetesEvent(event, false)
		if err != nil {
			_ = k.Warnf("Error while mapping event, %s.", err.Error())
			continue
		}

		log.Debugf("Sending event: %s", mappedEvent.String())
		sender.Event(mappedEvent)
	}

	return nil
}

func init() {
	core.RegisterCheck(kubernetesAPIEventsCheckName, KubernetesAPIEventsFactory)
}
