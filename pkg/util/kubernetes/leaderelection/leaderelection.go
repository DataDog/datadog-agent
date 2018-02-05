// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"flag"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"os"
	"time"
	"sync"
	"github.com/DataDog/datadog-agent/pkg/util/retry"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/apimachinery/pkg/util/wait"
)

const(
	defaultLeaderLeaseDuration = 60 * time.Second
	defaultLeaseName      = "datadog-leader-election"


)
// LeaderData represents information about the current leader
type LeaderData struct {
	Name string `json:"name"`
	sync.RWMutex
}

type LeaderEngine struct {
	initRetry retry.Retrier

	HolderIdentity string
	LeaderData *LeaderData
	LeaseDuration time.Duration
	LeaseName string
	coreClient *corev1.CoreV1Client
	leaderElector *leaderelection.LeaderElector
}

var (
	clientTimeout              = 20 * time.Second
)

var globalLeaderEngine *LeaderEngine

func newLeaderEngine(holderIdentity string) *LeaderEngine{
	return &LeaderEngine{
		HolderIdentity: holderIdentity,
		LeaderData: &LeaderData{},
		LeaseDuration:defaultLeaderLeaseDuration,
		LeaseName: defaultLeaseName,
	}
}

func GetLeaderEngine() (*LeaderEngine, error){
	if globalLeaderEngine == nil {
		holderIdentity, _ := os.Hostname() // TODO get hostname from DD
		globalLeaderEngine = newLeaderEngine(holderIdentity)
		globalLeaderEngine.initRetry.SetupRetrier(&retry.Config{
			Name:"leaderElection",
			AttemptMethod: globalLeaderEngine.init,
			Strategy: retry.RetryCount,
			RetryCount: 10,
			RetryDelay: 30 * time.Second,
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
	le.coreClient, err = GetClient()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err)
		return err
	}

	// check if we can get endpoints.
	_, resourceErr := le.coreClient.Endpoints(metav1.NamespaceDefault).List(metav1.ListOptions{Limit: 1})
	if resourceErr != nil {
		log.Errorf("Cannot retrieve endpoints from the %s namespace", metav1.NamespaceDefault)
		return resourceErr
	}

	le.leaderElector, err = NewElection(le.LeaseName, le.HolderIdentity, metav1.NamespaceDefault, le.LeaseDuration, le.LeaderData.callbackFunc, le.coreClient)
	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err.Error())
		return err
	}
	log.Debug("Kubernetes official client successfully initialized")
	return nil
}

func (ld *LeaderData) callbackFunc(str string){
	ld.Lock()
	ld.Name = str
	ld.Unlock()
}

// RunElection runs an election given an leader elector. Doesn't return.
// The passed LeaderElector embeds callback functions that are triggered to handle the different states of the process.
func (le *LeaderEngine) StartLeaderElection() {
	log.Info("Starting Leader Election process...")
	go wait.Forever(le.leaderElector.Run, 0)
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
	le.LeaderData.RLock()
	defer le.LeaderData.RUnlock()
	return le.LeaderData.Name
}

// StartLeaderElection is the main method that triggers the Leader Election process.
// It is a go routine that runs asynchronously with the agent and leverages the official Leader Election
// See the doc https://godoc.org/k8s.io/client-go/tools/leaderelection
//func StartLeaderElection(leaseDuration time.Duration) error {
//	kubeClient, err := GetClient()
//
//	if err != nil {
//		log.Errorf("Not Able to set up a client for the Leader Election: %s", err.Error())
//		return err
//	}
//
//	id, errHostname := os.Hostname()
//
//	if errHostname != nil {
//		log.Errorf("Cannot get OS hostname. Not setting up the Leader Election: %s", errHostname.Error())
//		return errHostname
//	}
//
//	var leaderLeaseDuration time.Duration
//	if leaseDuration.Seconds() > 0 {
//		leaderLeaseDuration = leaseDuration
//	} else {
//		log.Debugf("Leader Lease duration not properly set, defaulting to 60 seconds")
//		leaderLeaseDuration = defaultLeaderLeaseDuration
//	}
//	return nil
//
//}

func init(){
	// Avoid logging glog from the API Server.
	flag.Lookup("stderrthreshold").Value.Set("FATAL")
	flag.Parse()
}
