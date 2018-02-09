// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"encoding/json"
	"time"

	log "github.com/cihub/seelog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ld "k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

func (le *LeaderEngine) getCurrentLeader(electionId, namespace string) (string, *v1.Endpoints, error) {
	endpoint, err := le.coreClient.Endpoints(namespace).Get(electionId, metav1.GetOptions{})
	if err != nil {
		return "", nil, err
	}

	val, found := endpoint.Annotations[rl.LeaderElectionRecordAnnotationKey]
	if !found {
		return "", endpoint, nil
	}

	electionRecord := rl.LeaderElectionRecord{}
	if err := json.Unmarshal([]byte(val), &electionRecord); err != nil {
		return "", nil, err
	}
	return electionRecord.HolderIdentity, endpoint, err
}

// newElection creates an election.
// If `namespace`/`election` does not exist, it is created.
func (le *LeaderEngine) newElection(electionId, namespace string, ttl time.Duration) (*ld.LeaderElector, error) {
	// We first want to check if the Endpoint the Leader Election is based on exists.
	_, err := le.coreClient.Endpoints(namespace).Get(electionId, metav1.GetOptions{})

	if err != nil {
		if errors.IsNotFound(err) == false {
			return nil, err
		}
		_, err = le.coreClient.Endpoints(namespace).Create(&v1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name: electionId,
			},
			TypeMeta: metav1.TypeMeta{
				Kind: "Endpoints",
			},
		})
		if err != nil && !errors.IsConflict(err) {
			return nil, err
		}
	}

	currentLeader, endpoint, err := le.getCurrentLeader(electionId, namespace)
	if err != nil {
		return nil, err
	}
	log.Debugf("Current registered leader is %q", currentLeader)

	callbacks := ld.LeaderCallbacks{
		OnNewLeader: func(identity string) {
			le.currentHolderMutex.Lock()
			le.currentHolderIdentity = identity
			le.currentHolderMutex.Unlock()
			log.Infof("Currently new leader %q", identity)
		},
		OnStartedLeading: func(stop <-chan struct{}) {
			log.Infof("Leading as %q ...", le.HolderIdentity)
		},
		// OnStoppedLeading shouldn't be called unless the election is lost
		OnStoppedLeading: func() {
			le.currentHolderMutex.Lock()
			le.currentHolderIdentity = ""
			le.currentHolderMutex.Unlock()
			log.Warnf("Stop leading %q", le.HolderIdentity)
		},
	}

	eventSource := v1.EventSource{
		Component: "leader-elector",
		Host:      le.HolderIdentity,
	}
	broadcaster := record.NewBroadcaster()
	evRec := broadcaster.NewRecorder(runtime.NewScheme(), eventSource)
	resourceLockConfig := rl.ResourceLockConfig{
		Identity:      le.HolderIdentity,
		EventRecorder: evRec,
	}
	leaderElectorInterface, err := rl.New(
		rl.EndpointsResourceLock,
		endpoint.ObjectMeta.Namespace,
		endpoint.ObjectMeta.Name,
		le.coreClient,
		resourceLockConfig,
	)
	if err != nil {
		return nil, err
	}

	electionConfig := ld.LeaderElectionConfig{
		Lock:          leaderElectorInterface,
		LeaseDuration: ttl,
		RenewDeadline: ttl / 2,
		RetryPeriod:   ttl / 4,
		Callbacks:     callbacks,
	}
	return ld.NewLeaderElector(electionConfig)
}
