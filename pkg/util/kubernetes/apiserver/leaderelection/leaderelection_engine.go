// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package leaderelection

import (
	"context"
	"encoding/json"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ld "k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (le *LeaderEngine) getCurrentLeader() (string, *v1.ConfigMap, error) {
	configMap, err := le.coreClient.ConfigMaps(le.LeaderNamespace).Get(context.TODO(), le.LeaseName, metav1.GetOptions{})
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
	_, err := le.coreClient.ConfigMaps(le.LeaderNamespace).Get(context.TODO(), le.LeaseName, metav1.GetOptions{})

	if err != nil {
		if errors.IsNotFound(err) == false {
			return nil, err
		}

		_, err = le.coreClient.ConfigMaps(le.LeaderNamespace).Create(context.TODO(), &v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind: "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: le.LeaseName,
			},
		}, metav1.CreateOptions{})
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
			le.updateLeaderIdentity(identity)
			le.reportLeaderMetric(identity == le.HolderIdentity)
			log.Infof("New leader %q", identity)
		},
		OnStartedLeading: func(ctx context.Context) {
			le.updateLeaderIdentity(le.HolderIdentity)
			le.reportLeaderMetric(true)
			le.notify()
			log.Infof("Started leading as %q...", le.HolderIdentity)
		},
		// OnStoppedLeading shouldn't be called unless the election is lost. This could happen if
		// we lose connection to the apiserver for the duration of the lease.
		OnStoppedLeading: func() {
			le.updateLeaderIdentity("")
			le.reportLeaderMetric(false)
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
		le.coordClient,
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

// updateLeaderIdentity sets leaderIdentity
func (le *LeaderEngine) updateLeaderIdentity(identity string) {
	le.leaderIdentityMutex.Lock()
	defer le.leaderIdentityMutex.Unlock()
	le.leaderIdentity = identity
}

// reportLeaderMetric updates the label of the leader metric on every leadership change
func (le *LeaderEngine) reportLeaderMetric(isLeader bool) {
	// We want to make sure only one (the latest) context is exposed for this metric
	// Delete previous run metric
	le.leaderMetric.Delete(metrics.JoinLeaderValue, "false")
	le.leaderMetric.Delete(metrics.JoinLeaderValue, "true")

	le.leaderMetric.Set(1.0, metrics.JoinLeaderValue, strconv.FormatBool(isLeader))
}

// notify sends a notification to subscribers when the current process becomes leader.
// notify is a simplistic notifier but can be extended to send different notification
// kinds (leadership acquisition / loss) in the future if needed.
func (le *LeaderEngine) notify() {
	le.m.Lock()
	defer le.m.Unlock()

	for _, s := range le.subscribers {
		s <- struct{}{}
	}
}
