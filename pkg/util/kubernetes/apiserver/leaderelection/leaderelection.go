// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	"context"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	defaultLeaderLeaseDuration = 60 * time.Second
	defaultLeaseName           = "datadog-leader-election"
	clientTimeout              = 2 * time.Second
)

var (
	globalLeaderEngine *LeaderEngine
)

// LeaderEngine is a structure for the LeaderEngine client to run leader election
// on Kubernetes clusters
type LeaderEngine struct {
	initRetry retry.Retrier

	running bool
	m       sync.Mutex
	once    sync.Once

	HolderIdentity      string
	LeaseDuration       time.Duration
	LeaseName           string
	LeaderNamespace     string
	coreClient          corev1.CoreV1Interface
	ServiceName         string
	leaderIdentityMutex sync.RWMutex
	leaderElector       *leaderelection.LeaderElector

	// leaderIdentity is the HolderIdentity of the current leader.
	leaderIdentity string
}

func newLeaderEngine() *LeaderEngine {
	return &LeaderEngine{
		LeaseName:       defaultLeaseName,
		LeaderNamespace: common.GetResourcesNamespace(),
		ServiceName:     config.Datadog.GetString("cluster_agent.kubernetes_service_name"),
	}
}

// ResetGlobalLeaderEngine is a helper to remove the current LeaderEngine global
// It is ONLY to be used for tests
func ResetGlobalLeaderEngine() {
	globalLeaderEngine = nil
}

// GetLeaderEngine returns a leader engine client with default parameters.
func GetLeaderEngine() (*LeaderEngine, error) {
	return GetCustomLeaderEngine("", defaultLeaderLeaseDuration)
}

// GetCustomLeaderEngine wraps GetLeaderEngine for testing purposes.
func GetCustomLeaderEngine(holderIdentity string, ttl time.Duration) (*LeaderEngine, error) {
	if globalLeaderEngine == nil {
		globalLeaderEngine = newLeaderEngine()
		globalLeaderEngine.HolderIdentity = holderIdentity
		globalLeaderEngine.LeaseDuration = ttl
		globalLeaderEngine.initRetry.SetupRetrier(&retry.Config{
			Name:          "leaderElection",
			AttemptMethod: globalLeaderEngine.init,
			Strategy:      retry.RetryCount,
			RetryCount:    10,
			RetryDelay:    30 * time.Second,
		})
	}
	err := globalLeaderEngine.initRetry.TriggerRetry()
	if err != nil {
		log.Debugf("Leader Election init error: %s", err)
		return nil, err
	}
	return globalLeaderEngine, nil
}

func (le *LeaderEngine) init() error {
	var err error

	if le.HolderIdentity == "" {
		le.HolderIdentity, err = os.Hostname()
		if err != nil {
			log.Debugf("cannot get hostname: %s", err)
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

	apiClient, err := apiserver.GetAPIClient()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err)
		return err
	}

	le.coreClient = apiClient.Cl.CoreV1().(*corev1.CoreV1Client)

	// check if we can get ConfigMap.
	_, err = le.coreClient.ConfigMaps(le.LeaderNamespace).Get(defaultLeaseName, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) == false {
		log.Errorf("Cannot retrieve ConfigMap from the %s namespace: %s", le.LeaderNamespace, err)
		return err
	}

	le.leaderElector, err = le.newElection()
	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err)
		return err
	}
	log.Debugf("Leader Engine for %q successfully initialized", le.HolderIdentity)
	return nil
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

	le.once.Do(
		func() {
			go le.runLeaderElection()
		},
	)

	timeoutDuration := clientTimeout * 2
	timeout := time.After(timeoutDuration)
	tick := time.NewTicker(time.Millisecond * 500)
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
			return fmt.Errorf("leader election still not running, timeout after %s", timeoutDuration)
		}
	}
}

func (le *LeaderEngine) runLeaderElection() {
	for {
		log.Infof("Starting leader election process for %q...", le.HolderIdentity)

		le.leaderElector.Run(context.Background())
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
// The result is not cached.
func (le *LeaderEngine) GetLeaderIP() (string, error) {
	leaderName := le.GetLeader()
	if leaderName == "" || leaderName == le.HolderIdentity {
		return "", nil
	}

	endpointList, err := le.coreClient.Endpoints(le.LeaderNamespace).Get(le.ServiceName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	target, err := apiserver.SearchTargetPerName(endpointList, leaderName)
	if err != nil {
		return "", err
	}
	return target.IP, nil
}

// IsLeader returns true if the last observed leader was this client else returns false.
func (le *LeaderEngine) IsLeader() bool {
	return le.GetLeader() == le.HolderIdentity
}

// GetLeaderElectionRecord is used in for the Flare and for the Status commands.
func GetLeaderElectionRecord() (leaderDetails rl.LeaderElectionRecord, err error) {
	var led rl.LeaderElectionRecord
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return led, err
	}

	c := client.Cl.CoreV1()

	leaderNamespace := common.GetResourcesNamespace()
	leaderElectionCM, err := c.ConfigMaps(leaderNamespace).Get(defaultLeaseName, metav1.GetOptions{})
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
