// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	log "github.com/cihub/seelog"

	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"time"
	"os"
)
// LeaderData represents information about the current leader
type LeaderData struct {
	Name string `json:"name"`
}
var (
	leader = &LeaderData{}
)

func getClient() (*corev1.CoreV1Client, error){

	config, err := rest.InClusterConfig()
	config.Timeout = 5 * time.Second
	config.Insecure = true

	if err != nil {
		return nil, err
	}
	coreClient, err := corev1.NewForConfig(config)

	return coreClient, err
}
func GetLeader() string {
	return leader.Name
}

func StartLeaderelection() error {

	kubeClient, err := getClient()
	if err != nil {
		log.Errorf("Not Able to set up a client for the Leader Election: %s", err.Error())
		return err
	}

	fn := func(str string) {
		leader.Name = str
		// To remove
		log.Debugf("The leader is %s: ", leader.Name)
	}
	electionID := "datadog-leader-election"
	id, errHostname := os.Hostname()

	if err != nil {
		log.Error("Cannot get OS hostname. Not setting up the Leader Election: %s", errHostname.Error())
		return err
	}

	e, err := NewElection(electionID, metav1.NamespaceDefault, id, 10* time.Second, fn, kubeClient)

	if err != nil {
		log.Errorf("Could not initialize the Leader Election process: %s", err.Error())
	}
	go RunElection(e)
	return nil

}
