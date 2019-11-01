// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/batcher"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	collectors "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topology_collectors"
	"github.com/StackVista/stackstate-agent/pkg/topology"
	"github.com/StackVista/stackstate-agent/pkg/util/kubernetes/apiserver"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
	"sync"
	"time"
)

const (
	kubernetesAPITopologyCheckName = "kubernetes_api_topology"
)

// TopologyCheck grabs events from the API server.
type TopologyCheck struct {
	CommonCheck
	instance *TopologyConfig
}

// Configure parses the check configuration and init the check.
func (t *TopologyCheck) Configure(config, initConfig integration.Data) error {
	err := t.ConfigureKubeApiCheck(config)
	if err != nil {
		return err
	}

	err = t.instance.parse(config)
	if err != nil {
		_ = log.Error("could not parse the config for the API topology check")
		return err
	}

	log.Debugf("Running config %s", config)
	return nil
}

/*
	cluster -> map cluster -> component

	node -> map node -> component
				     -> cluster relation
		   component <- container correlator
			relation <-

	pod -> map pod 	  		 -> component
							 -> node relation
		container correlator <- map func container -> component
												   -> relation

	service -> map service -> component
						   -> endpoints as identifiers
						   -> pod relation

	component -> publish component
	relation -> publish relation
*/
// Run executes the check.
func (t *TopologyCheck) Run() error {
	// initialize kube api check
	err := t.InitKubeApiCheck()
	if err != nil {
		return err
	}

	// Running the event collection.
	if !t.instance.CollectTopology {
		return nil
	}

	// set the check "instance id" for snapshots
	t.instance.CheckID = kubernetesAPITopologyCheckName

	var instanceClusterType ClusterType
	switch openshiftPresence := t.ac.DetectOpenShiftAPILevel(); openshiftPresence {
	case apiserver.OpenShiftAPIGroup, apiserver.OpenShiftOAPI:
		instanceClusterType = OpenShift
	case apiserver.NotOpenShift:
		instanceClusterType = Kubernetes
	}
	t.instance.Instance = topology.Instance{Type: string(instanceClusterType), URL: t.instance.ClusterName}

	// start the topology snapshot with the batch-er
	batcher.GetBatcher().SubmitStartSnapshot(t.instance.CheckID, t.instance.Instance)

	// Make a channel for each of the relations to avoid passing data down into all the functions
	containerCorrelationChannel := make(chan *collectors.ContainerCorrelation)
	serviceCorrelationChannel := make(chan *collectors.IngressCorrelation)

	// make a channel that is responsible for publishing components and relations
	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	errChannel := make(chan error)
	waitGroupChannel := make(chan int)

	t.setupTopologyReceiver(componentChannel, relationChannel, errChannel, waitGroupChannel)
	commonClusterCollector := collectors.NewClusterTopologyCollector(t.instance.Instance, t.ac)
	clusterCollectors := []collectors.ClusterTopologyCollector{
		// Register Cluster Component Collector
		collectors.NewClusterCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register ConfigMap Component Collector
		collectors.NewConfigMapCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register CronJob Component Collector
		collectors.NewCronJobCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register DaemonSet Component Collector
		collectors.NewDaemonSetCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Deployment Component Collector
		collectors.NewDeploymentCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Ingress Component Collector
		collectors.NewIngressCollector(
			componentChannel,
			relationChannel,
			serviceCorrelationChannel,
			commonClusterCollector,
		),
		// Register Job Component Collector
		collectors.NewJobCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Node Component Collector
		collectors.NewNodeCollector(
			componentChannel,
			relationChannel,
			containerCorrelationChannel,
			commonClusterCollector,
		),
		// Register Persistent Volume Component Collector
		collectors.NewPersistentVolumeCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Pod Component Collector
		collectors.NewPodCollector(
			componentChannel,
			relationChannel,
			containerCorrelationChannel,
			commonClusterCollector,
		),
		// Register ReplicaSet Component Collector
		collectors.NewReplicaSetCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Service Component Collector
		collectors.NewServiceCollector(
			componentChannel,
			relationChannel,
			serviceCorrelationChannel,
			commonClusterCollector,
		),
		// Register StatefulSet Component Collector
		collectors.NewStatefulSetCollector(
			componentChannel,
			commonClusterCollector,
		),
	}

	t.runClusterCollectors(clusterCollectors, waitGroupChannel, errChannel)
	// get all the containers
	batcher.GetBatcher().SubmitStopSnapshot(t.instance.CheckID, t.instance.Instance)
	batcher.GetBatcher().SubmitComplete(t.instance.CheckID)

	log.Debugf("Topology Check for cluster: %s completed successfully", t.instance.ClusterName)
	// close all the created channels
	close(componentChannel)
	close(relationChannel)
	close(errChannel)
	close(waitGroupChannel)

	return nil
}

// sets up the receiver that handles the component and relation channel and publishes it to StackState
func (t *TopologyCheck) setupTopologyReceiver(componentChannel <-chan *topology.Component, relationChannel <-chan *topology.Relation,
	errorChannel <-chan error, waitGroupChannel <-chan int) {
	go func() {
	loop:
		for {
			select {
			case component := <- componentChannel:
				log.Debugf("Publishing StackState %s component for %s: %v", component.Type.Name, component.ExternalID, component.JSONString())
				batcher.GetBatcher().SubmitComponent(t.instance.CheckID, t.instance.Instance, *component)
			case relation := <- relationChannel:
				log.Debugf("Publishing StackState %s relation %s->%s", relation.Type.Name, relation.SourceID, relation.TargetID)
				batcher.GetBatcher().SubmitRelation(t.instance.CheckID, t.instance.Instance, *relation)
			case err := <- errorChannel:
				_ = log.Error(err)
			case <- waitGroupChannel:
				log.Debug("All collectors have been finished their work, continuing to publish data to StackState")
				break loop
			default:
				// no message received, continue looping
			}
		}
	}()
}

// runs all of the cluster collectors, notify the wait groups and submit errors to the error channel
func (t *TopologyCheck) runClusterCollectors(clusterCollectors []collectors.ClusterTopologyCollector, waitGroupChannel chan<- int, errorChannel chan<- error) {
	// set up a WaitGroup to wait for the concurrent topology gathering of the functions below
	var waitGroup sync.WaitGroup

	for _, collector := range clusterCollectors {
		// add this collector to the wait group
		runCollector(collector, errorChannel, &waitGroup)
	}

	timeout := time.Duration(t.instance.CollectTimeout) * time.Minute
	log.Debugf("Waiting for Cluster Collectors to Finish")
	_ = waitTimeout(&waitGroup, timeout)
	// submit the end to the topology listener
	waitGroupChannel <- 1
}

// waitTimeout waits for the waitgroup for the specified max timeout.
// Returns true if waiting timed out.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	wgChan := make(chan struct{})
	go func() {
		defer close(wgChan)
		wg.Wait()
	}()
	select {
	case <-wgChan:
		log.Debugf("WaitGroup for Cluster Collectors has finished, stopping topology publish loop")
		return false // completed normally
	case <-time.After(timeout):
		_ = log.Warn("WaitGroup for Cluster Collectors did not finish in time, stopping topology publish loop")
		return true // timed out
	}
}

// runCollector
func runCollector(collector collectors.ClusterTopologyCollector, errorChannel chan<- error, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		log.Debugf("Starting cluster topology collector: %s\n", collector.GetName())
		err := collector.CollectorFunction()
		if err != nil {
			errorChannel <- err
		}
		// mark this collector as complete
		log.Debugf("Finished cluster topology collector: %s\n", collector.GetName())
		wg.Done()
	}()
}

// KubernetesASFactory is exported for integration testing.
func KubernetesApiTopologyFactory() check.Check {
	return &TopologyCheck{
		CommonCheck: CommonCheck{
			CheckBase: core.NewCheckBase(kubernetesAPITopologyCheckName),
		},
		instance: &TopologyConfig{},
	}
}

func init() {
	core.RegisterCheck(kubernetesAPITopologyCheckName, KubernetesApiTopologyFactory)
}
