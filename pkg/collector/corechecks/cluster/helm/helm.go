// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package helm

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkName               = "helm"
	maximumWaitForAPIServer = 10 * time.Second
	informerSyncTimeout     = 60 * time.Second
)

func init() {
	core.RegisterCheck(checkName, factory)
}

type helmStorage string

const (
	k8sSecrets    helmStorage = "secret"
	k8sConfigmaps helmStorage = "configmap"
)

// HelmCheck collects information about the Helm releases deployed in the
// cluster. The check works for Helm installations configured to use Kubernetes
// secrets or configmaps as the storage. K8s secrets are the default in Helm v3.
// Helm v2 used config maps by default. Ref:
// https://helm.sh/docs/faq/changes_since_helm2/#secrets-as-the-default-storage-driver
type HelmCheck struct {
	core.CheckBase
	releases          map[helmStorage]map[string]*release
	releasesMutex     sync.Mutex
	runLeaderElection bool
}

func factory() check.Check {
	helmCheck := &HelmCheck{
		CheckBase:         core.NewCheckBase(checkName),
		releases:          make(map[helmStorage]map[string]*release),
		runLeaderElection: !config.IsCLCRunner(),
	}

	for _, storageDriver := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		helmCheck.releases[storageDriver] = make(map[string]*release)
	}

	return helmCheck
}

// Configure configures the Helm check
func (hc *HelmCheck) Configure(config, initConfig integration.Data, source string) error {
	hc.BuildID(config, initConfig)

	err := hc.CommonConfigure(config, source)
	if err != nil {
		return err
	}

	err = hc.CommonConfigure(initConfig, source)
	if err != nil {
		return err
	}

	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()

	apiClient, err := apiserver.WaitForAPIClient(apiCtx)
	if err != nil {
		return err
	}

	return hc.setupInformers(apiClient.InformerFactory)
}

// Run executes the check
func (hc *HelmCheck) Run() error {
	sender, err := hc.GetSender()
	if err != nil {
		return err
	}
	defer sender.Commit()

	if hc.runLeaderElection {
		isCurrentLeader, errLeader := isLeader()
		if errLeader != nil {
			return errLeader
		}

		if !isCurrentLeader {
			log.Debugf("Not leader. Skipping the Helm check")
			return nil
		}
	}

	hc.releasesMutex.Lock()
	defer hc.releasesMutex.Unlock()

	for _, storageDriver := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		for _, rel := range hc.releases[storageDriver] {
			sender.Gauge("helm.release", 1, "", helmTags(rel, storageDriver))
		}
	}

	return nil
}

func (hc *HelmCheck) setupInformers(sharedInformerFactory informers.SharedInformerFactory) error {
	stopCh := make(chan struct{})

	secretInformer := sharedInformerFactory.Core().V1().Secrets()
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    hc.addSecret,
		DeleteFunc: hc.deleteSecret,
		UpdateFunc: hc.updateSecret,
	})
	go secretInformer.Informer().Run(stopCh)

	configmapInformer := sharedInformerFactory.Core().V1().ConfigMaps()
	configmapInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    hc.addConfigmap,
		DeleteFunc: hc.deleteConfigmap,
		UpdateFunc: hc.updateConfigmap,
	})
	go configmapInformer.Informer().Run(stopCh)

	return apiserver.SyncInformers(
		map[apiserver.InformerName]cache.SharedInformer{
			"helm-secrets":    secretInformer.Informer(),
			"helm-configmaps": configmapInformer.Informer(),
		},
		informerSyncTimeout,
	)
}

func helmTags(release *release, storageDriver helmStorage) []string {
	tags := []string{
		fmt.Sprintf("helm_release:%s", release.Name),
		fmt.Sprintf("helm_namespace:%s", release.Namespace),
		fmt.Sprintf("helm_revision:%d", release.Version),
		fmt.Sprintf("helm_storage:%s", storageDriver),
	}

	// I've found releases without a chart reference. Not sure if it's due to
	// failed deployments, bugs in Helm, etc.
	if release.Chart != nil && release.Chart.Metadata != nil {
		tags = append(
			tags,
			fmt.Sprintf("helm_chart_name:%s", release.Chart.Metadata.Name),
			fmt.Sprintf("helm_chart_version:%s", release.Chart.Metadata.Version),
			fmt.Sprintf("helm_app_version:%s", release.Chart.Metadata.AppVersion),
		)
	}

	if release.Info != nil {
		tags = append(tags, fmt.Sprintf("helm_status:%s", release.Info.Status))
	}

	return tags
}

func (hc *HelmCheck) addSecret(obj interface{}) {
	secret, ok := obj.(*v1.Secret)
	if !ok {
		log.Warnf("Expected secret, got: %v", obj)
		return
	}

	if !isManagedByHelm(secret) {
		return
	}

	hc.addRelease(string(secret.Data["release"]), k8sSecrets)
}

func (hc *HelmCheck) deleteSecret(obj interface{}) {
	secret, ok := obj.(*v1.Secret)
	if !ok {
		log.Warnf("Expected secret, got: %v", obj)
		return
	}

	if !isManagedByHelm(secret) {
		return
	}

	hc.deleteRelease(string(secret.Data["release"]), k8sSecrets)
}

func (hc *HelmCheck) updateSecret(_, obj interface{}) {
	hc.addSecret(obj)
}

func (hc *HelmCheck) addConfigmap(obj interface{}) {
	configmap, ok := obj.(*v1.ConfigMap)
	if !ok {
		log.Warnf("Expected configmap, got: %v", obj)
		return
	}

	if !isManagedByHelm(configmap) {
		return
	}

	hc.addRelease(configmap.Data["release"], k8sConfigmaps)
}

func (hc *HelmCheck) deleteConfigmap(obj interface{}) {
	configmap, ok := obj.(*v1.ConfigMap)
	if !ok {
		log.Warnf("Expected configmap, got: %v", obj)
		return
	}

	if !isManagedByHelm(configmap) {
		return
	}

	hc.deleteRelease(configmap.Data["release"], k8sConfigmaps)
}

func (hc *HelmCheck) updateConfigmap(_, obj interface{}) {
	hc.addConfigmap(obj)
}

func (hc *HelmCheck) addRelease(encodedRelease string, storageDriver helmStorage) {
	decodedRelease, err := decodeRelease(encodedRelease)
	if err != nil {
		log.Debugf("error while decoding Helm release: %s", err)
		return
	}

	hc.releasesMutex.Lock()
	defer hc.releasesMutex.Unlock()
	hc.releases[storageDriver][decodedRelease.ID()] = decodedRelease
}

func (hc *HelmCheck) deleteRelease(encodedRelease string, storageDriver helmStorage) {
	decodedRelease, err := decodeRelease(encodedRelease)
	if err != nil {
		log.Debugf("error while decoding Helm release: %s", err)
		return
	}

	hc.releasesMutex.Lock()
	defer hc.releasesMutex.Unlock()
	delete(hc.releases[storageDriver], decodedRelease.ID())
}

func isManagedByHelm(object metav1.Object) bool {
	return object.GetLabels()["owner"] == "helm"
}

func isLeader() (bool, error) {
	if !config.Datadog.GetBool("leader_election") {
		return false, errors.New("leader election not enabled. The check will not run")
	}

	_, errLeader := cluster.RunLeaderElection()
	if errLeader != nil {
		if errLeader == apiserver.ErrNotLeader {
			return false, nil
		}

		return false, errLeader
	}

	return true, nil
}
