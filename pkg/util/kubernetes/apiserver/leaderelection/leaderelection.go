// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	log "github.com/cihub/seelog"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"

	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/config"
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

	once sync.Once

	HolderIdentity string
	LeaseDuration  time.Duration
	LeaseName      string
	coreClient     *corev1.CoreV1Client
	leaderElector  *leaderelection.LeaderElector

	currentHolderIdentity string
	currentHolderMutex    sync.RWMutex
}

func newLeaderEngine() *LeaderEngine {
	return &LeaderEngine{
		LeaseName: defaultLeaseName,
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
		log.Debugf("Init error: %s", err)
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
	if leaseDuration != 0 {
		le.LeaseDuration = time.Duration(leaseDuration) * time.Second
	}

	if le.LeaseDuration == 0 {
		le.LeaseDuration = defaultLeaderLeaseDuration
	}
	log.Debugf("LeaderLeaseDuration: %s", le.LeaseDuration.String())

	le.coreClient, err = apiserver.GetCoreV1Client()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err)
		return err
	}

	// check if we can get ConfigMap.
	_, err = le.coreClient.ConfigMaps(metav1.NamespaceDefault).Get(defaultLeaseName, metav1.GetOptions{})
	if err != nil && errors.IsNotFound(err) == false {
		log.Errorf("Cannot retrieve ConfigMap from the %s namespace: %s", metav1.NamespaceDefault, err)
		return err
	}

	le.leaderElector, err = le.newElection(le.LeaseName, metav1.NamespaceDefault, le.LeaseDuration)
	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err)
		return err
	}
	log.Debugf("Leader Engine for %q successfully initialized", le.HolderIdentity)
	return nil
}

func (le *LeaderEngine) startLeaderElection() {
	log.Infof("Starting Leader Election process for %q ...", le.HolderIdentity)
	go wait.Forever(le.leaderElector.Run, 0)
}

// EnsureLeaderElectionRuns start the Leader election process if not already started,
// return nil if the process is effectively running
func (le *LeaderEngine) EnsureLeaderElectionRuns() error {
	var leaderIdentity string

	le.once.Do(le.startLeaderElection)
	timeoutDuration := clientTimeout * 2
	timeout := time.After(timeoutDuration)
	tick := time.NewTicker(time.Millisecond * 500)
	for {
		select {
		case <-tick.C:
			leaderIdentity = le.CurrentLeaderName()
			if leaderIdentity != "" {
				log.Infof("Leader Election run, current leader is %q", leaderIdentity)
				return nil
			}
			log.Tracef("Leader identity is unset")

		case <-timeout:
			return fmt.Errorf("leader election still not running, timeout after %s", timeoutDuration.String())
		}
	}
}

// CurrentLeaderName is the main interface that can be called to fetch the name of the current leader.
func (le *LeaderEngine) CurrentLeaderName() string {
	le.currentHolderMutex.RLock()
	defer le.currentHolderMutex.RUnlock()

	return le.currentHolderIdentity
}

// IsLeader return bool if the current LeaderEngine is the leader
func (le *LeaderEngine) IsLeader() bool {
	return le.CurrentLeaderName() == le.HolderIdentity
}

// GetLeaderDetails is used in for the Flare and for the Status commands.
func GetLeaderDetails() (leaderDetails rl.LeaderElectionRecord, err error) {
	var led rl.LeaderElectionRecord
	c, err := apiserver.GetCoreV1Client()
	if err != nil {
		return led, err
	}
	leaderElectionCM, err := c.ConfigMaps(metav1.NamespaceDefault).Get(defaultLeaseName, metav1.GetOptions{})
	if err != nil {
		return led, err
	}
	log.Infof("LeaderElection cm is %q", leaderElectionCM)
	annotation, found := leaderElectionCM.Annotations[rl.LeaderElectionRecordAnnotationKey]
	if !found {
		return led, apiserver.ErrNotFound
	}
	bytes := []byte(annotation)
	err = json.Unmarshal(bytes, &led)
	if err != nil {
		return led, err
	}
	return led, nil
}

func init() {
	// Avoid logging glog from the k8s.io package
	flag.Lookup("stderrthreshold").Value.Set("FATAL")
	//Convinces goflags that we have called Parse() to avoid noisy logs.
	//OSS Issue: kubernetes/kubernetes#17162.
	flag.CommandLine.Parse([]string{})
}
