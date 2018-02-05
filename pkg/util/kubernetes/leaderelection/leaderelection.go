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
)

// LeaderData represents information about the current leader
type LeaderData struct {
	Name string `json:"name"`
}

var (
	leader                     = &LeaderData{}
	datadogLeaderElection      = "datadog-leader-election"
	defaultLeaderLeaseDuration = 60 * time.Second
	clientTimeout              = 20 * time.Second
)

// GetClient returns an official client
func getClient() (*corev1.CoreV1Client, error) {
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
func GetLeader() string {
	return leader.Name
}

// StartLeaderElection is the main method that triggers the Leader Election process.
// It is a go routine that runs asynchronously with the agent and leverages the official Leader Election
// See the doc https://godoc.org/k8s.io/client-go/tools/leaderelection
func StartLeaderElection(leaseDuration time.Duration) error {
	// Avoid logging glog from the API Server.
	flag.Lookup("stderrthreshold").Value.Set("FATAL")
	flag.Parse()

	kubeClient, err := getClient()

	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err.Error())
		return err
	}

	_, resourceErr := kubeClient.Endpoints(metav1.NamespaceDefault).List(metav1.ListOptions{Limit: 1})
	if resourceErr != nil {
		log.Errorf("cannot retrieve endpoints from the %s namespace", metav1.NamespaceDefault)
		return resourceErr
	}
	callbackFunc := func(str string) {
		leader.Name = str
	}
	id, errHostname := os.Hostname()

	if errHostname != nil {
		log.Error("Cannot get OS hostname. Not setting up the Leader Election: %s", errHostname.Error())
		return errHostname
	}

	var leaderLeaseDuration time.Duration
	if leaseDuration.Seconds() > 0 {
		leaderLeaseDuration = leaseDuration
	} else {
		log.Debugf("Leader Lease duration not properly set, defaulting to 60 seconds")
		leaderLeaseDuration = defaultLeaderLeaseDuration
	}

	e, err := NewElection(datadogLeaderElection, id, metav1.NamespaceDefault, leaderLeaseDuration, callbackFunc, kubeClient)

	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err.Error())
		return err
	}

	go RunElection(e)
	return nil

}
