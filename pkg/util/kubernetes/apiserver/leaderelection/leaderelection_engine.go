// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubeapiserver

package leaderelection

import (
	"encoding/json"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ld "k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"context"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (le *LeaderEngine) getCurrentLeader() (string, *v1.ConfigMap, error) {
	configMap, err := le.coreClient.ConfigMaps(le.LeaderNamespace).Get(le.LeaseName, metav1.GetOptions{})
	if err != nil {
		return "", nil, err
	}

	val, found := configMap.Annotations[rl.LeaderElectionRecordAnnotationKey]
	if !found {
		log.Debugf("The configmap/%s in the namespace %s doesn't have the annotation %q: no one is leading yet", le.LeaseName, le.LeaderNamespace, rl.LeaderElectionRecordAnnotationKey)
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
func (le *LeaderEngine) newElection() (*ld.LeaderElector, error) {
	// We first want to check if the ConfigMap the Leader Election is based on exists.
	_, err := le.coreClient.ConfigMaps(le.LeaderNamespace).Get(le.LeaseName, metav1.GetOptions{})

	if err != nil {
		if errors.IsNotFound(err) == false {
			return nil, err
		}

		_, err = le.coreClient.ConfigMaps(le.LeaderNamespace).Create(&v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind: "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: le.LeaseName,
			},
		})
		if err != nil && !errors.IsConflict(err) {
			return nil, err
		}
	}

	currentLeader, configMap, err := le.getCurrentLeader()
	if err != nil {
		return nil, err
	}
	log.Debugf("Current registered leader is %q, building leader elector %q as candidate", currentLeader, le.HolderIdentity)
	callbacks := ld.LeaderCallbacks{
		OnNewLeader: func(identity string) {
			le.leaderIdentityMutex.Lock()
			le.leaderIdentity = identity
			le.leaderIdentityMutex.Unlock()

			log.Infof("New leader %q", identity)
		},
		OnStartedLeading: func(ctx context.Context) {
			le.leaderIdentityMutex.Lock()
			le.leaderIdentity = le.HolderIdentity
			le.leaderIdentityMutex.Unlock()

			log.Infof("Started leading as %q...", le.HolderIdentity)
		},
		// OnStoppedLeading shouldn't be called unless the election is lost. This could happen if
		// we lose connection to the apiserver for the duration of the lease.
		OnStoppedLeading: func() {
			le.leaderIdentityMutex.Lock()
			le.leaderIdentity = ""
			le.leaderIdentityMutex.Unlock()

			log.Infof("Stopped leading %q", le.HolderIdentity)
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
		nil, // relying on CM so unnecessary.
		resourceLockConfig,
	)
	if err != nil {
		return nil, err
	}

	electionConfig := ld.LeaderElectionConfig{
		Lock:          leaderElectorInterface,
		LeaseDuration: le.LeaseDuration,
		RenewDeadline: le.LeaseDuration / 2,
		RetryPeriod:   le.LeaseDuration / 4,
		Callbacks:     callbacks,
	}
	return ld.NewLeaderElector(electionConfig)
}
