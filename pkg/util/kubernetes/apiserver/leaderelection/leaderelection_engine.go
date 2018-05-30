// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ld "k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

func (le *LeaderEngine) getCurrentLeader(electionId, namespace string) (string, *v1.ConfigMap, error) {
	configMap, err := le.coreClient.ConfigMaps(namespace).Get(electionId, metav1.GetOptions{})
	if err != nil {
		return "", nil, err
	}

	val, found := configMap.Annotations[rl.LeaderElectionRecordAnnotationKey]
	if !found {
		log.Debugf("The configmap/%s in the namespace %s doesn't have the annotation %q: no one is leading yet", electionId, namespace, rl.LeaderElectionRecordAnnotationKey)
		return "", configMap, nil
	}

	electionRecord := rl.LeaderElectionRecord{}
	if err := json.Unmarshal([]byte(val), &electionRecord); err != nil {
		return "", nil, err
	}
	return electionRecord.HolderIdentity, configMap, err
}

// newElection creates an election.
// If `namespace`/`election` does not exist, it is created.
func (le *LeaderEngine) newElection(electionId, namespace string, ttl time.Duration) (*ld.LeaderElector, error) {
	// We first want to check if the ConfigMap the Leader Election is based on exists.
	_, err := le.coreClient.ConfigMaps(namespace).Get(electionId, metav1.GetOptions{})

	if err != nil {
		if errors.IsNotFound(err) == false {
			return nil, err
		}

		_, err = le.coreClient.ConfigMaps(namespace).Create(&v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind: "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: electionId,
			},
		})
		if err != nil && !errors.IsConflict(err) {
			return nil, err
		}
	}

	currentLeader, configMap, err := le.getCurrentLeader(electionId, namespace)
	if err != nil {
		return nil, err
	}
	log.Debugf("Current registered leader is %q, building leader elector %q as candidate", currentLeader, le.HolderIdentity)

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
		rl.ConfigMapsResourceLock,
		configMap.ObjectMeta.Namespace,
		configMap.ObjectMeta.Name,
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
