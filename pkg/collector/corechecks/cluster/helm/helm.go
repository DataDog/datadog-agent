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

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster"
	"github.com/DataDog/datadog-agent/pkg/config"
	coreMetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	checkName               = "helm"
	serviceCheckName        = "helm.release_state"
	maximumWaitForAPIServer = 10 * time.Second
	defaultExtraSyncTimeout = 120 * time.Second
	defaultResyncInterval   = 10 * time.Minute
	labelSelector           = "owner=helm"
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
	instance          *checkConfig
	store             *releasesStore
	runLeaderElection bool
	eventsManager     *eventsManager
	informerFactory   informers.SharedInformerFactory
	startTS           time.Time
	once              sync.Once
}

type checkConfig struct {
	CollectEvents                  bool `yaml:"collect_events"`
	ExtraSyncTimeoutSeconds        int  `yaml:"extra_sync_timeout_seconds"`
	InformersResyncIntervalMinutes int  `yaml:"informers_resync_interval_minutes"`
}

// Parse parses the config and sets default values
func (cc *checkConfig) Parse(data []byte) error {
	// default values
	cc.CollectEvents = false

	return yaml.Unmarshal(data, cc)
}

func factory() check.Check {
	return &HelmCheck{
		CheckBase:         core.NewCheckBase(checkName),
		instance:          &checkConfig{},
		store:             newReleasesStore(),
		runLeaderElection: !config.IsCLCRunner(),
		eventsManager:     &eventsManager{},
	}
}

// Configure configures the Helm check
func (hc *HelmCheck) Configure(config, initConfig integration.Data, source string) error {
	hc.BuildID(config, initConfig)

	err := hc.CommonConfigure(initConfig, config, source)
	if err != nil {
		return err
	}

	if err = hc.instance.Parse(config); err != nil {
		return err
	}

	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()

	apiClient, err := apiserver.WaitForAPIClient(apiCtx)
	if err != nil {
		return err
	}

	hc.setSharedInformerFactory(apiClient)
	hc.startTS = time.Now()

	return nil
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

	hc.once.Do(func() {
		// We sync the informers here in Run to avoid blocking
		// Configure for several seconds/minutes depending on the number of configmaps/secrets.
		if err = hc.setupInformers(); err != nil {
			log.Errorf("Couldn't setup informers: %v", err)
		}
	})

	for _, storageDriver := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		for _, rel := range hc.store.getAll(storageDriver) {
			sender.Gauge("helm.release", 1, "", tagsForMetricsAndEvents(rel, storageDriver, true))
		}
	}

	if hc.instance.CollectEvents {
		hc.eventsManager.sendEvents(sender)
	}

	hc.sendServiceCheck(sender)

	return nil
}

func (hc *HelmCheck) setupInformers() error {
	stopCh := make(chan struct{})

	secretInformer := hc.informerFactory.Core().V1().Secrets()
	secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    hc.addSecret,
		DeleteFunc: hc.deleteSecret,
		UpdateFunc: hc.updateSecret,
	})
	go secretInformer.Informer().Run(stopCh)

	configmapInformer := hc.informerFactory.Core().V1().ConfigMaps()
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
		hc.getExtraSyncTimeout(),
	)
}

func (hc *HelmCheck) setSharedInformerFactory(apiClient *apiserver.APIClient) {
	hc.informerFactory = informers.NewSharedInformerFactoryWithOptions(
		apiClient.Cl,
		hc.getInformersResyncPeriod(),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = labelSelector
		}),
	)
}

func tagsForMetricsAndEvents(release *release, storageDriver helmStorage, includeRevision bool) []string {
	tags := tagsForServiceCheck(release, storageDriver)

	if includeRevision {
		tags = append(tags, fmt.Sprintf("helm_revision:%d", release.Version))
	}

	// I've found releases without a chart reference. Not sure if it's due to
	// failed deployments, bugs in Helm, etc.
	if release.Chart != nil && release.Chart.Metadata != nil {
		tags = append(
			tags,
			fmt.Sprintf("helm_chart_version:%s", release.Chart.Metadata.Version),
			fmt.Sprintf("helm_app_version:%s", release.Chart.Metadata.AppVersion),
		)
	}

	if release.Info != nil {
		tags = append(tags, fmt.Sprintf("helm_status:%s", release.Info.Status))
	}

	return tags
}

// tagsForServiceCheck returns the tags needed for the service check which
// are the ones that don't change between revisions
func tagsForServiceCheck(release *release, storageDriver helmStorage) []string {
	tags := []string{
		fmt.Sprintf("helm_release:%s", release.Name),
		fmt.Sprintf("helm_storage:%s", storageDriver),
		fmt.Sprintf("kube_namespace:%s", release.Namespace),

		// "helm_namespace" is just an alias for "kube_namespace".
		// "kube_namespace" is a better name and consistent with the rest of
		// checks, but in the first release of the check we had "helm_namespace"
		// so we need to keep it for backwards-compatibility.
		fmt.Sprintf("helm_namespace:%s", release.Namespace),
	}

	if release.Chart != nil && release.Chart.Metadata != nil {
		tags = append(tags, fmt.Sprintf("helm_chart_name:%s", release.Chart.Metadata.Name))
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

	hc.addRelease(string(secret.Data["release"]), secret.GetCreationTimestamp(), k8sSecrets)
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

func (hc *HelmCheck) updateSecret(old, new interface{}) {
	oldSecret, ok := old.(*v1.Secret)
	if !ok {
		log.Warnf("Expected secret, got: %T", old)
		return
	}

	newSecret, ok := new.(*v1.Secret)
	if !ok {
		log.Warnf("Expected secret, got: %T", old)
		return
	}

	if oldSecret.ResourceVersion == newSecret.ResourceVersion {
		return
	}

	hc.addSecret(newSecret)
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

	hc.addRelease(configmap.Data["release"], configmap.GetCreationTimestamp(), k8sConfigmaps)
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

func (hc *HelmCheck) updateConfigmap(old, new interface{}) {
	oldConfigmap, ok := old.(*v1.ConfigMap)
	if !ok {
		log.Warnf("Expected configmap, got: %T", old)
		return
	}

	newConfigmap, ok := new.(*v1.ConfigMap)
	if !ok {
		log.Warnf("Expected configmap, got: %T", old)
		return
	}

	if oldConfigmap.ResourceVersion == newConfigmap.ResourceVersion {
		return
	}

	hc.addConfigmap(newConfigmap)
}

func (hc *HelmCheck) addRelease(encodedRelease string, creationTS metav1.Time, storageDriver helmStorage) {
	decodedRelease, err := decodeRelease(encodedRelease)
	if err != nil {
		log.Debugf("error while decoding Helm release: %s", err)
		return
	}

	needToEmitEvent := hc.instance.CollectEvents && creationTS.After(hc.startTS)

	if needToEmitEvent {
		if previous := hc.store.get(decodedRelease.namespacedName(), decodedRelease.revision(), storageDriver); previous != nil {
			hc.eventsManager.addEventForUpdatedRelease(previous, decodedRelease, storageDriver)
		} else {
			hc.eventsManager.addEventForNewRelease(decodedRelease, storageDriver)
		}
	}

	hc.store.add(decodedRelease, storageDriver)
}

func (hc *HelmCheck) deleteRelease(encodedRelease string, storageDriver helmStorage) {
	decodedRelease, err := decodeRelease(encodedRelease)
	if err != nil {
		log.Debugf("error while decoding Helm release: %s", err)
		return
	}

	moreRevisionsLeft := hc.store.delete(decodedRelease, storageDriver)

	// When a release is deleted, all its revisions are deleted at the same
	// time. To avoid generating many events with the same info, we just emit
	// one when there are no more revisions left.
	if hc.instance.CollectEvents && !moreRevisionsLeft {
		hc.eventsManager.addEventForDeletedRelease(decodedRelease, storageDriver)
	}
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

func (hc *HelmCheck) sendServiceCheck(sender aggregator.Sender) {
	for _, storageDriver := range []helmStorage{k8sConfigmaps, k8sSecrets} {
		for _, rel := range hc.store.getLatestRevisions(storageDriver) {
			tags := tagsForServiceCheck(rel, storageDriver)

			if rel.Info != nil && rel.Info.Status == "failed" {
				sender.ServiceCheck(serviceCheckName, coreMetrics.ServiceCheckCritical, "", tags, "Release in \"failed\" state")
			} else {
				sender.ServiceCheck(serviceCheckName, coreMetrics.ServiceCheckOK, "", tags, "")
			}
		}
	}
}

func (hc *HelmCheck) getExtraSyncTimeout() time.Duration {
	if hc.instance != nil && hc.instance.ExtraSyncTimeoutSeconds > 0 {
		return time.Duration(hc.instance.ExtraSyncTimeoutSeconds) * time.Second
	}
	return defaultExtraSyncTimeout
}

func (hc *HelmCheck) getInformersResyncPeriod() time.Duration {
	if hc.instance != nil && hc.instance.InformersResyncIntervalMinutes > 0 {
		return time.Duration(hc.instance.InformersResyncIntervalMinutes) * time.Minute
	}
	return defaultResyncInterval
}
