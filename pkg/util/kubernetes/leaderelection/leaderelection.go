// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"flag"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"k8s.io/client-go/tools/leaderelection"
)

const (
	defaultLeaderLeaseDuration = 60 * time.Second
	defaultLeaseName           = "datadog-leader-election"
	clientTimeout              = 2 * time.Second
)

var (
	globalLeaderEngine        *LeaderEngine
	globalHolderIdentity      string
	globalLeaderLeaseDuration = 0 * time.Second
)

type LeaderEngine struct {
	initRetry retry.Retrier

	HolderIdentity string
	LeaseDuration  time.Duration
	LeaseName      string
	coreClient     *corev1.CoreV1Client
	leaderElector  *leaderelection.LeaderElector
	stopCh         chan struct{}
}

func newLeaderEngine() *LeaderEngine {
	return &LeaderEngine{
		LeaseName: defaultLeaseName,
		stopCh:    make(chan struct{}),
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
		log.Errorf("Cannot retrieve endpoints from the %s namespace", metav1.NamespaceDefault)
		return err
	}

	le.leaderElector, err = NewElection(le.LeaseName, le.HolderIdentity, metav1.NamespaceDefault, le.LeaseDuration, le.coreClient)
	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err.Error())
		return err
	}
	log.Debug("Kubernetes official client successfully initialized")
	return nil
}

// RunElection runs an election given an leader elector. Doesn't return.
// The passed LeaderElector embeds callback functions that are triggered to handle the different states of the process.
func (le *LeaderEngine) StartLeaderElection() {
	log.Infof("Starting Leader Election process for %q ...", le.HolderIdentity)
	go func() {
		for {
			select {
			case <-le.stopCh:
				log.Warnf("Stop the Leader Election process for %q", le.HolderIdentity)
				return
			default:
				log.Infof("Leader Election running...")
				le.leaderElector.Run()
			}
		}
	}()
}

// GetClient returns an official client
func GetClient() (*corev1.CoreV1Client, error) {
	var k8sconfig *rest.Config
	var err error

	cfgPath := config.Datadog.GetString("kubernetes_kubeconfig_path")
	if cfgPath == "" {
		k8sconfig, err = rest.InClusterConfig()
		if err != nil {
			log.Debug("Can't create a config for the official client from the service account's token")
			return nil, err
		}
	} else {
		// use the current context in kubeconfig
		k8sconfig, err = clientcmd.BuildConfigFromFlags("", cfgPath)
		if err != nil {
			log.Debug("Can't create a config for the official client from the configured path to the kubeconfig")
			return nil, err
		}
	}

	k8sconfig.Timeout = clientTimeout
	coreClient, err := corev1.NewForConfig(k8sconfig)

	return coreClient, err
}

// GetLeader is the main interface that can be called to fetch the name of the current leader.
func (le *LeaderEngine) GetLeader() string {
	return le.leaderElector.GetLeader()
}

// GetLeader is the main interface that can be called to fetch the name of the current leader.
func (le *LeaderEngine) IsLeader() bool {
	return le.leaderElector.IsLeader()
}

func init() {
	// Avoid logging glog from the k8s.io package
	flag.Lookup("stderrthreshold").Value.Set("FATAL")
	flag.Parse()
}
