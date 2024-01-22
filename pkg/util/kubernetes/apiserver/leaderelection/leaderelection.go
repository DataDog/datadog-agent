// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package leaderelection provides functions related with the leader election
// mechanism offered in Kubernetes.
package leaderelection

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"
	configmaplock "github.com/DataDog/datadog-agent/internal/third_party/client-go/tools/leaderelection/resourcelock"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"golang.org/x/mod/semver"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	coordinationv1 "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	defaultLeaderLeaseDuration = 60 * time.Second
	getLeaderTimeout           = 10 * time.Second
)

var globalLeaderEngine *LeaderEngine

// LeaderEngine is a structure for the LeaderEngine client to run leader election
// on Kubernetes clusters
type LeaderEngine struct {
	ctx       context.Context
	initRetry retry.Retrier

	running bool
	m       sync.Mutex
	once    sync.Once

	subscribers         []chan struct{}
	HolderIdentity      string
	LeaseDuration       time.Duration
	LeaseName           string
	LeaderNamespace     string
	coreClient          corev1.CoreV1Interface
	coordClient         coordinationv1.CoordinationV1Interface
	ServiceName         string
	leaderIdentityMutex sync.RWMutex
	leaderElector       *leaderelection.LeaderElector
	lockType            string

	// leaderIdentity is the HolderIdentity of the current leader.
	leaderIdentity string

	// leaderMetric indicates whether this instance is leader
	leaderMetric telemetry.Gauge
}

func newLeaderEngine(ctx context.Context) *LeaderEngine {
	return &LeaderEngine{
		ctx:             ctx,
		LeaseName:       config.Datadog.GetString("leader_lease_name"),
		LeaderNamespace: common.GetResourcesNamespace(),
		ServiceName:     config.Datadog.GetString("cluster_agent.kubernetes_service_name"),
		leaderMetric:    metrics.NewLeaderMetric(),
		subscribers:     []chan struct{}{},
		LeaseDuration:   defaultLeaderLeaseDuration,
	}
}

// ResetGlobalLeaderEngine is a helper to remove the current LeaderEngine global
// It is ONLY to be used for tests
func ResetGlobalLeaderEngine() {
	globalLeaderEngine = nil
	telemetryComponent.GetCompatComponent().Reset()
}

// Initialize initializes the leader engine
func (le *LeaderEngine) initialize() *retry.Error {
	err := globalLeaderEngine.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("Leader Election init error: %s", err)
	}
	return err
}

// GetLeaderEngine returns an initialized leader engine.
func GetLeaderEngine() (*LeaderEngine, error) {
	if globalLeaderEngine == nil {
		return nil, fmt.Errorf("Global Leader Engine was not created")
	}
	err := globalLeaderEngine.initialize()
	if err != nil {
		return nil, err
	}
	return globalLeaderEngine, nil
}

// CreateGlobalLeaderEngine returns a non initialized leader engine client
func CreateGlobalLeaderEngine(ctx context.Context) *LeaderEngine {
	if globalLeaderEngine == nil {
		globalLeaderEngine = newLeaderEngine(ctx)
		globalLeaderEngine.initRetry.SetupRetrier(&retry.Config{ //nolint:errcheck
			Name:              "leaderElection",
			AttemptMethod:     globalLeaderEngine.init,
			Strategy:          retry.Backoff,
			InitialRetryDelay: 1 * time.Second,
			MaxRetryDelay:     5 * time.Minute,
		})
	}
	return globalLeaderEngine
}

func (le *LeaderEngine) init() error {
	var err error

	if le.HolderIdentity == "" {
		le.HolderIdentity, err = common.GetSelfPodName()
		if err != nil {
			log.Debugf("cannot get pod name: %s", err)
			return err
		}
	}
	log.Debugf("Init LeaderEngine with HolderIdentity: %q", le.HolderIdentity)

	leaseDuration := config.Datadog.GetInt("leader_lease_duration")
	if leaseDuration > 0 {
		le.LeaseDuration = time.Duration(leaseDuration) * time.Second
	} else {
		le.LeaseDuration = defaultLeaderLeaseDuration
	}
	log.Debugf("LeaderLeaseDuration: %s", le.LeaseDuration.String())

	// Using GetAPIClient (no retry) as LeaderElection is already wrapped in a retrier
	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err)
		return err
	}

	serverVersion, err := common.KubeServerVersion(apiClient.DiscoveryCl, 10*time.Second)
	if err == nil && semver.IsValid(serverVersion.String()) && semver.Compare(serverVersion.String(), "v1.14.0") < 0 {
		log.Warn("[DEPRECATION WARNING] DataDog will drop support of Kubernetes older than v1.14. Please update to a newer version to ensure proper functionality and security.")
	}

	le.coreClient = apiClient.Cl.CoreV1()
	le.coordClient = apiClient.Cl.CoordinationV1()

	usingLease, err := CanUseLeases(apiClient.DiscoveryCl)
	if err != nil {
		log.Errorf("Unable to retrieve available resources: %v", err)
		return err
	}
	if usingLease {
		log.Debugf("leader election will use Leases to store the leader token")
		le.lockType = rl.LeasesResourceLock
	} else {
		// for kubernetes <= 1.13
		log.Debugf("leader election will use ConfigMaps to store the leader token")
		le.lockType = configmaplock.ConfigMapsResourceLock
	}
	le.leaderElector, err = le.newElection()
	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err)
		return err
	}
	log.Debugf("Leader Engine for %q successfully initialized", le.HolderIdentity)
	return nil
}

// StartLeaderElectionRun starts the runLeaderElection once
func (le *LeaderEngine) StartLeaderElectionRun() {
	le.once.Do(
		func() {
			go le.runLeaderElection()
		},
	)
}

// EnsureLeaderElectionRuns start the Leader election process if not already running,
// return nil if the process is effectively running
func (le *LeaderEngine) EnsureLeaderElectionRuns() error {
	le.m.Lock()
	defer le.m.Unlock()

	if le.running {
		log.Debugf("Currently Leader: %t. Leader identity: %q", le.IsLeader(), le.GetLeader())
		return nil
	}

	le.StartLeaderElectionRun()

	timeout := time.After(getLeaderTimeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		log.Tracef("Waiting for new leader identity...")
		select {
		case <-tick.C:
			leaderIdentity := le.GetLeader()
			if leaderIdentity != "" {
				log.Infof("Leader election running, current leader is %q", leaderIdentity)
				le.running = true
				return nil
			}
		case <-timeout:
			return fmt.Errorf("leader election still not running, timeout after %s", getLeaderTimeout)
		}
	}
}

func (le *LeaderEngine) runLeaderElection() {
	for {
		log.Infof("Starting leader election process for %q...", le.HolderIdentity)
		le.leaderElector.Run(le.ctx)
		log.Info("Leader election lost")
	}
}

// GetLeader returns the identity of the last observed leader or returns the empty string if
// no leader has yet been observed.
func (le *LeaderEngine) GetLeader() string {
	le.leaderIdentityMutex.RLock()
	defer le.leaderIdentityMutex.RUnlock()

	return le.leaderIdentity
}

// GetLeaderIP returns the IP the leader can be reached at, assuming its
// identity is its pod name. Returns empty if we are the leader.
// The result is cached and will not return an error if the leader does not exist anymore.
func (le *LeaderEngine) GetLeaderIP() (string, error) {
	leaderName := le.GetLeader()
	if leaderName == "" || leaderName == le.HolderIdentity {
		return "", nil
	}

	cacheKey := "ip://" + leaderName
	if ip, found := cache.Cache.Get(cacheKey); found {
		return ip.(string), nil
	}

	endpointList, err := le.coreClient.Endpoints(le.LeaderNamespace).Get(context.TODO(), le.ServiceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	target, err := apiserver.SearchTargetPerName(endpointList, leaderName)
	if err != nil {
		return "", err
	}
	cache.Cache.Set(cacheKey, target.IP, 1*time.Hour)

	return target.IP, nil
}

// IsLeader returns true if the last observed leader was this client else returns false.
func (le *LeaderEngine) IsLeader() bool {
	return le.GetLeader() == le.HolderIdentity
}

// Subscribe allows any component to receive a notification
// when the current process becomes leader.
// Calling Subscribe is optional, use IsLeader if the client doesn't need an event-based approach.
func (le *LeaderEngine) Subscribe() <-chan struct{} {
	c := make(chan struct{}, 5) // buffered channel to avoid blocking in case of stuck subscriber

	le.m.Lock()
	le.subscribers = append(le.subscribers, c)
	le.m.Unlock()

	return c
}

func detectLeases(client discovery.DiscoveryInterface) (bool, error) {
	resourceList, err := client.ServerResourcesForGroupVersion("coordination.k8s.io/v1")
	if kerrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, actualResource := range resourceList.APIResources {
		if actualResource.Name == "leases" {
			return true, nil
		}
	}

	return false, nil
}

// CanUseLeases returns if leases can be used for leader election. If the resource is defined in the config
// It uses it. Otherwise it uses the discovery client for leader election.
func CanUseLeases(client discovery.DiscoveryInterface) (bool, error) {
	resourceType := config.Datadog.GetString("leader_election_default_resource")
	if resourceType == "lease" || resourceType == "leases" {
		return true, nil
	} else if resourceType == "configmap" || resourceType == "configmaps" {
		return false, nil
	}

	if resourceType != "" {
		log.Warnf("Unknown resource lock for leader election [%s]. Using the discovery client to select the lock", resourceType)
	}

	return detectLeases(client)
}

func getLeaseLeaderElectionRecord(client coordinationv1.CoordinationV1Interface) (rl.LeaderElectionRecord, error) {
	var empty rl.LeaderElectionRecord
	lease, err := client.Leases(common.GetResourcesNamespace()).Get(context.TODO(), config.Datadog.GetString("leader_lease_name"), metav1.GetOptions{})
	if err != nil {
		return empty, err
	}
	log.Debugf("LeaderElection lease is %#v", lease)
	record := rl.LeaseSpecToLeaderElectionRecord(&lease.Spec)
	return *record, nil
}

func getConfigMapLeaderElectionRecord(client corev1.CoreV1Interface) (rl.LeaderElectionRecord, error) {
	var led rl.LeaderElectionRecord
	leaderElectionCM, err := client.ConfigMaps(common.GetResourcesNamespace()).Get(context.TODO(), config.Datadog.GetString("leader_lease_name"), metav1.GetOptions{})
	if err != nil {
		return led, err
	}
	log.Debugf("LeaderElection cm is %#v", leaderElectionCM)
	annotation, found := leaderElectionCM.Annotations[rl.LeaderElectionRecordAnnotationKey]
	if !found {
		return led, apiserver.ErrNotFound
	}
	err = json.Unmarshal([]byte(annotation), &led)
	if err != nil {
		return led, err
	}
	return led, nil
}

// GetLeaderElectionRecord is used in for the Flare and for the Status commands.
func GetLeaderElectionRecord() (leaderDetails rl.LeaderElectionRecord, err error) {
	var led rl.LeaderElectionRecord
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return led, err
	}
	usingLease, err := CanUseLeases(client.DiscoveryCl)
	if err != nil {
		return led, err
	}
	if usingLease {
		return getLeaseLeaderElectionRecord(client.Cl.CoordinationV1())
	}
	return getConfigMapLeaderElectionRecord(client.Cl.CoreV1())
}
