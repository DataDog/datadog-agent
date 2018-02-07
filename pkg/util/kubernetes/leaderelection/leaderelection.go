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

	log "github.com/cihub/seelog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
)

const (
	defaultLeaderLeaseDuration = 60 * time.Second
	defaultLeaseName           = "datadog-leader-election"
	clientTimeout              = 2 * time.Second
)

var (
	globalHolderIdentity      string
	globalLeaderEngine        *LeaderEngine
	globalLeaderLeaseDuration = 0 * time.Second
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

// SetLeaderLeaseDuration is a helper to set the current LeaderLeaseDuration global
// It is ONLY to be used for tests
func SetLeaderLeaseDuration(ttl time.Duration) {
	globalLeaderLeaseDuration = ttl
}

// SetHolderIdentify is a helper to set the current holderIdentify global
// It is ONLY to be used for tests
func SetHolderIdentify(holderIdentity string) {
	globalHolderIdentity = holderIdentity
}

// GetLeaderEngine returns the leader engine client
func GetLeaderEngine() (*LeaderEngine, error) {
	if globalLeaderEngine == nil {
		globalLeaderEngine = newLeaderEngine()
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

	if globalHolderIdentity == "" {
		globalHolderIdentity, err = os.Hostname()
		if err != nil {
			log.Debugf("cannot get hostname: %s", err)
			return err
		}
	}
	le.HolderIdentity = globalHolderIdentity
	log.Debugf("HolderIdentity is %q", globalHolderIdentity)

	leaseDuration := config.Datadog.GetInt("leader_lease_duration")
	if leaseDuration != 0 {
		globalLeaderLeaseDuration = time.Duration(leaseDuration) * time.Second
	}

	if globalLeaderLeaseDuration == 0 {
		globalLeaderLeaseDuration = defaultLeaderLeaseDuration
	}
	le.LeaseDuration = globalLeaderLeaseDuration
	log.Debugf("LeaderLeaseDuration is %s", globalLeaderLeaseDuration.String())

	le.coreClient, err = GetClient()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err)
		return err
	}

	// check if we can get endpoints.
	_, err = le.coreClient.Endpoints(metav1.NamespaceDefault).List(metav1.ListOptions{Limit: 1})
	if err != nil {
		log.Errorf("Cannot retrieve endpoints from the %s namespace: %s", metav1.NamespaceDefault, err)
		return err
	}

	le.leaderElector, err = le.newElection(le.LeaseName, metav1.NamespaceDefault, le.LeaseDuration)
	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err)
		return err
	}
	log.Debug("Leader Engine successfully initialized")
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

// GetClient returns an official Kubernetes core v1 client
func GetClient() (*corev1.CoreV1Client, error) {
	var k8sConfig *rest.Config
	var err error

	cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
	if cfgPath == "" {
		k8sConfig, err = rest.InClusterConfig()
		if err != nil {
			log.Debug("Can't create a config for the official client from the service account's token: %s", err)
			return nil, err
		}
	} else {
		// use the current context in kubeconfig
		k8sConfig, err = clientcmd.BuildConfigFromFlags("", cfgPath)
		if err != nil {
			log.Debug("Can't create a config for the official client from the configured path to the kubeconfig: %s, ", cfgPath, err)
			return nil, err
		}
	}

	k8sConfig.Timeout = clientTimeout
	coreClient, err := corev1.NewForConfig(k8sConfig)

	return coreClient, err
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

func init() {
	// Avoid logging glog from the k8s.io package
	flag.Lookup("stderrthreshold").Value.Set("FATAL")
	flag.Parse()
}
