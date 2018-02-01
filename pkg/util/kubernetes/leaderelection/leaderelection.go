// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	log "github.com/cihub/seelog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"

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

func getClient() (*corev1.CoreV1Client, error) {

	config, err := rest.InClusterConfig()
	config.Timeout = clientTimeout

	if err != nil {
		log.Debug("Can't create official client")
		return nil, err
	}
	coreClient, err := corev1.NewForConfig(config)

	return coreClient, err
}

// GetLeader is the main interface that can be called to fetch the name of the current leader.
func GetLeader() string {
	return leader.Name
}

// StartLeaderElection is the main method that triggers the Leader Election process.
// It is a go routine that runs asynchronously with the agent and leverages the official Leader Election
// See the doc https://godoc.org/k8s.io/client-go/tools/leaderelection
func StartLeaderElection(leaseDuration int) error {

	kubeClient, err := getClient()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err.Error())
		return err
	}

	callbackFunc := func(str string) {
		leader.Name = str
	}
	id, errHostname := os.Hostname()

	if errHostname != nil {
		log.Error("Cannot get OS hostname. Not setting up the Leader Election: %s", errHostname.Error())
		return errHostname
	}
	if leaseDuration != 0 {
		leaseDuration = time.Second * leaseDuration
	} else {
		log.Debugf("Leader Lease duration not properly set, defaulting to 60 seconds")
		leaseDuration = defaultLeaderLeaseDuration
	}

	e, err := NewElection(datadogLeaderElection, id, metav1.NamespaceDefault, leaseDuration, callbackFunc, kubeClient)

	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err.Error())
		return err
	}

	go RunElection(e)
	return nil

}
