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

	coordv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientcoord "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	ld "k8s.io/client-go/tools/leaderelection"
	rl "k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	configmaplock "github.com/DataDog/datadog-agent/internal/third_party/client-go/tools/leaderelection/resourcelock"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func NewReleaseLock(lockType string, ns string, name string, coreClient corev1.CoreV1Interface, coordinationClient clientcoord.CoordinationV1Interface, rlc rl.ResourceLockConfig) (rl.Interface, error) {
	if lockType == configmaplock.ConfigMapsResourceLock {
		return &configmaplock.ConfigMapLock{
			ConfigMapMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      name,
			},
			Client:     coreClient,
			LockConfig: rlc,
		}, nil
	}

	return rl.New(lockType, ns, name, coreClient, coordinationClient, rlc)
}

func (le *LeaderEngine) getCurrentLeaderLease() (string, error) {
	lease, err := le.coordClient.Leases(le.LeaderNamespace).Get(context.TODO(), le.LeaseName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	// leases do not store the leader election data in annotations but directly in the specs
	leader := lease.Spec.HolderIdentity
	if leader == nil {
		log.Debugf("The lease/%s in the namespace %s doesn't have the field leader in its spec: no one is leading yet", le.LeaseName, le.LeaderNamespace)
		return "", nil
	}

	return *leader, err

}

func (le *LeaderEngine) getCurrentLeaderConfigMap() (string, error) {
	configMap, err := le.coreClient.ConfigMaps(le.LeaderNamespace).Get(context.TODO(), le.LeaseName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	val, found := configMap.Annotations[rl.LeaderElectionRecordAnnotationKey]
	if !found {
		log.Debugf("The configmap/%s in the namespace %s doesn't have the annotation %q: no one is leading yet", le.LeaseName, le.LeaderNamespace, rl.LeaderElectionRecordAnnotationKey)
		return "", nil
	}

	electionRecord := rl.LeaderElectionRecord{}
	if err := json.Unmarshal([]byte(val), &electionRecord); err != nil {
		return "", err
	}
	return electionRecord.HolderIdentity, err
}

func (le *LeaderEngine) getCurrentLeader() (string, error) {
	if le.lockType == rl.LeasesResourceLock {
		return le.getCurrentLeaderLease()
	}

	return le.getCurrentLeaderConfigMap()
}

func (le *LeaderEngine) CreateLeaderTokenIfNotExists() error {
	if le.lockType == rl.LeasesResourceLock {
		_, err := le.coordClient.Leases(le.LeaderNamespace).Get(context.TODO(), le.LeaseName, metav1.GetOptions{})

		if err != nil {
			if !errors.IsNotFound(err) {
				return err
			}

			_, err = le.coordClient.Leases(le.LeaderNamespace).Create(context.TODO(), &coordv1.Lease{
				TypeMeta: metav1.TypeMeta{
					Kind: "Lease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      le.LeaseName,
					Namespace: le.LeaderNamespace,
				},
			}, metav1.CreateOptions{})
			if err != nil && !errors.IsConflict(err) {
				return err
			}
		}
	}
	_, err := le.coreClient.ConfigMaps(le.LeaderNamespace).Get(context.TODO(), le.LeaseName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
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
			return err
		}
	}
	return err
}

// newElection creates an election.
// If `namespace`/`election` does not exist, it is created.
func (le *LeaderEngine) newElection() (*ld.LeaderElector, error) {
	// We first want to check if the ConfigMap the Leader Election is based on exists.
	if err := le.CreateLeaderTokenIfNotExists(); err != nil {
		return nil, err
	}

	currentLeader, err := le.getCurrentLeader()
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

	leaderElectorInterface, err := NewReleaseLock(
		le.lockType,
		le.LeaderNamespace,
		le.LeaseName,
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
