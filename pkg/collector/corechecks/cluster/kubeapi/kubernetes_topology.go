// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
// +build kubeapiserver

package kubeapi

import (
	"github.com/StackVista/stackstate-agent/pkg/autodiscovery/integration"
	"github.com/StackVista/stackstate-agent/pkg/collector/check"
	core "github.com/StackVista/stackstate-agent/pkg/collector/corechecks"
	collectors "github.com/StackVista/stackstate-agent/pkg/collector/corechecks/cluster/topologycollectors"
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
	instance  *TopologyConfig
	submitter TopologySubmitter
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

// SetSubmitter sets the topology submitter for the Topology Check
func (t *TopologyCheck) SetSubmitter(submitter TopologySubmitter) {
	t.submitter = submitter
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

	// set up the batcher for this instance
	t.submitter = NewBatchTopologySubmitter(t.instance.CheckID, t.instance.Instance)

	// start the topology snapshot with the batch-er
	t.submitter.SubmitStartSnapshot()

	// create a wait group for all the collectors
	var waitGroup sync.WaitGroup

	// Make a channel for each of the relations to avoid passing data down into all the functions
	nodeIdentifierCorrelationChannel := make(chan *collectors.NodeIdentifierCorrelation)
	containerCorrelationChannel := make(chan *collectors.ContainerCorrelation)

	// make a channel that is responsible for publishing components and relations
	componentChannel := make(chan *topology.Component)
	relationChannel := make(chan *topology.Relation)
	errChannel := make(chan error)
	waitGroupChannel := make(chan bool)

	clusterTopologyCommon := collectors.NewClusterTopologyCommon(t.instance.Instance, t.ac)
	commonClusterCollector := collectors.NewClusterTopologyCollector(clusterTopologyCommon)
	clusterCollectors := []collectors.ClusterTopologyCollector{
		// Register Cluster Component Collector
		collectors.NewClusterCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Node Component Collector
		collectors.NewNodeCollector(
			componentChannel,
			relationChannel,
			nodeIdentifierCorrelationChannel,
			commonClusterCollector,
		),
		// Register ConfigMap Component Collector
		collectors.NewConfigMapCollector(
			componentChannel,
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
		// Register ReplicaSet Component Collector
		collectors.NewReplicaSetCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register StatefulSet Component Collector
		collectors.NewStatefulSetCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Persistent Volume Component Collector
		collectors.NewPersistentVolumeCollector(
			componentChannel,
			commonClusterCollector,
		),
		// Register Pod Component Collector
		collectors.NewPodCollector(
			componentChannel,
			relationChannel,
			containerCorrelationChannel,
			commonClusterCollector,
		),
		// Register Service Component Collector
		collectors.NewServiceCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Ingress Component Collector
		collectors.NewIngressCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register Job Component Collector
		collectors.NewJobCollector(
			componentChannel,
			relationChannel,
			commonClusterCollector,
		),
		// Register CronJob Component Collector
		collectors.NewCronJobCollector(
			componentChannel,
			commonClusterCollector,
		),
	}

	commonClusterCorrelator := collectors.NewClusterTopologyCorrelator(clusterTopologyCommon)
	clusterCorrelators := []collectors.ClusterTopologyCorrelator{
		// Register Container -> Node Identifier Correlator
		collectors.NewContainerCorrelator(
			componentChannel,
			relationChannel,
			nodeIdentifierCorrelationChannel,
			containerCorrelationChannel,
			commonClusterCorrelator,
		),
	}

	// starts all the cluster collectors and correlators
	t.RunClusterCollectors(clusterCollectors, clusterCorrelators, &waitGroup, errChannel)

	// receive all the components, will return once the wait group notifies
	t.WaitForTopology(componentChannel, relationChannel, errChannel, &waitGroup, waitGroupChannel)

	t.submitter.SubmitStopSnapshot()
	t.submitter.SubmitComplete()

	log.Infof("Topology Check for cluster: %s completed successfully", t.instance.ClusterName)
	// close all the created channels
	close(componentChannel)
	close(relationChannel)
	close(errChannel)
	close(waitGroupChannel)

	return nil
}

// sets up the receiver that handles the component and relation channel and publishes it to StackState, returns when all the collectors have finished or the timeout was reached.
func (t *TopologyCheck) WaitForTopology(componentChannel <-chan *topology.Component, relationChannel <-chan *topology.Relation,
	errorChannel <-chan error, waitGroup *sync.WaitGroup, waitGroupChannel chan bool) {
	log.Debugf("Waiting for Cluster Collectors to Finish")
	go func() {
	loop:
		for {
			select {
			case component := <-componentChannel:
				t.submitter.SubmitComponent(component)
			case relation := <-relationChannel:
				t.submitter.SubmitRelation(relation)
			case err := <-errorChannel:
				t.submitter.HandleError(err)
			case timedOut := <-waitGroupChannel:
				if timedOut {
					_ = log.Warn("WaitGroup for Cluster Collectors did not finish in time, stopping topology publish loop")
				} else {
					log.Debug("All collectors have been finished their work, continuing to publish data to StackState")
				}

				break loop // timed out
			default:
				// no message received, continue looping
			}
		}
	}()

	timeout := time.Duration(t.instance.CollectTimeout) * time.Minute
	log.Debugf("Waiting for Cluster Collectors to Finish")
	waitGroupChannel <- waitTimeout(waitGroup, timeout)
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
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

// runs all of the cluster collectors, notify the wait groups and submit errors to the error channel
func (t *TopologyCheck) RunClusterCollectors(clusterCollectors []collectors.ClusterTopologyCollector, clusterCorrelators []collectors.ClusterTopologyCorrelator, waitGroup *sync.WaitGroup, errorChannel chan<- error) {
	waitGroup.Add(len(clusterCollectors))
	waitGroup.Add(len(clusterCorrelators))
	go func() {
		for _, collector := range clusterCollectors {
			// add this collector to the wait group
			runCollector(collector, errorChannel, waitGroup)
		}
	}()
	go func() {
		for _, correlator := range clusterCorrelators {
			// add this collector to the wait group
			runCorrelator(correlator, errorChannel, waitGroup)
		}
	}()
}

// runCollector
func runCollector(collector collectors.ClusterTopologyCollector, errorChannel chan<- error, wg *sync.WaitGroup) {
	log.Debugf("Starting cluster topology collector: %s\n", collector.GetName())
	err := collector.CollectorFunction()
	if err != nil {
		errorChannel <- err
	}
	// mark this collector as complete
	log.Debugf("Finished cluster topology collector: %s\n", collector.GetName())
	wg.Done()
}

// runCorrelator
func runCorrelator(correlator collectors.ClusterTopologyCorrelator, errorChannel chan<- error, wg *sync.WaitGroup) {
	log.Debugf("Starting cluster topology correlator: %s\n", correlator.GetName())
	err := correlator.CorrelateFunction()
	if err != nil {
		errorChannel <- err
	}
	// mark this collector as complete
	log.Debugf("Finished cluster topology correlator: %s\n", correlator.GetName())
	wg.Done()
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
