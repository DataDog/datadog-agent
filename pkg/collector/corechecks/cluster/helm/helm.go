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
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
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
	secretLister      v1.SecretLister
	configmapLister   v1.ConfigMapLister
	runLeaderElection bool
}

var helmSelector = labels.Set{"owner": "helm"}.AsSelector()

func factory() check.Check {
	return &HelmCheck{
		CheckBase:         core.NewCheckBase(checkName),
		runLeaderElection: !config.IsCLCRunner(),
	}
}

// Configure configures the Helm check
func (hc *HelmCheck) Configure(config, initConfig integration.Data, source string) error {
	apiCtx, apiCancel := context.WithTimeout(context.Background(), maximumWaitForAPIServer)
	defer apiCancel()

	apiClient, err := apiserver.WaitForAPIClient(apiCtx)
	if err != nil {
		return err
	}

	stopCh := make(chan struct{})
	apiClient.InformerFactory.Start(stopCh)

	secretInformer := apiClient.InformerFactory.Core().V1().Secrets()
	hc.secretLister = secretInformer.Lister()
	go secretInformer.Informer().Run(stopCh)

	configmapInformer := apiClient.InformerFactory.Core().V1().ConfigMaps()
	hc.configmapLister = configmapInformer.Lister()
	go configmapInformer.Informer().Run(stopCh)

	return apiserver.SyncInformers(
		map[apiserver.InformerName]cache.SharedInformer{
			"helm-secrets":    secretInformer.Informer(),
			"helm-configmaps": configmapInformer.Informer(),
		},
		informerSyncTimeout,
	)
}

// Run executes the check
func (hc *HelmCheck) Run() error {
	sender, err := aggregator.GetSender(hc.ID())
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

	releasesInSecrets, err := hc.releasesFromSecrets()
	if err != nil {
		return fmt.Errorf("error while getting Helm releases from secrets: %s", err)
	}

	for _, releaseInSecret := range releasesInSecrets {
		sender.Gauge("helm.release", 1, "", helmTags(releaseInSecret, k8sSecrets))
	}

	releasesInConfigMaps, err := hc.releasesFromConfigMaps()
	if err != nil {
		return fmt.Errorf("error while getting Helm releases from configmaps: %s", err)
	}

	for _, releaseInConfigMap := range releasesInConfigMaps {
		sender.Gauge("helm.release", 1, "", helmTags(releaseInConfigMap, k8sConfigmaps))
	}

	return nil
}

func helmTags(release *release, storageDriver helmStorage) []string {
	return []string{
		fmt.Sprintf("helm_release:%s", release.Name),
		fmt.Sprintf("helm_chart_name:%s", release.Chart.Metadata.Name),
		fmt.Sprintf("helm_namespace:%s", release.Namespace),
		fmt.Sprintf("helm_revision:%d", release.Version),
		fmt.Sprintf("helm_status:%s", release.Info.Status),
		fmt.Sprintf("helm_chart_version:%s", release.Chart.Metadata.Version),
		fmt.Sprintf("helm_app_version:%s", release.Chart.Metadata.AppVersion),
		fmt.Sprintf("helm_storage:%s", storageDriver),
	}
}

func (hc *HelmCheck) releasesFromSecrets() ([]*release, error) {
	secrets, err := hc.secretLister.List(helmSelector)
	if err != nil {
		return nil, err
	}

	var releases []*release

	for _, secret := range secrets {
		deployedRelease, err := decodeRelease(string(secret.Data["release"]))
		if err != nil {
			return nil, fmt.Errorf("error while decoding Helm release: %s", err)
		}
		releases = append(releases, deployedRelease)
	}

	return releases, nil
}

func (hc *HelmCheck) releasesFromConfigMaps() ([]*release, error) {
	configMaps, err := hc.configmapLister.List(helmSelector)
	if err != nil {
		return nil, err
	}

	var releases []*release

	for _, configMap := range configMaps {
		deployedRelease, err := decodeRelease(configMap.Data["release"])
		if err != nil {
			return nil, fmt.Errorf("error while decoding Helm release: %s", err)
		}
		releases = append(releases, deployedRelease)
	}

	return releases, nil
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
