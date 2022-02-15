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

// HelmCheck collects information about the Helm releases deployed in the
// cluster. The check only works for Helm installations configured to use
// Kubernetes secrets as the storage. K8s secrets are the default in Helm v3.
// Helm v2 used config maps by default. Ref:
// https://helm.sh/docs/faq/changes_since_helm2/#secrets-as-the-default-storage-driver
type HelmCheck struct {
	core.CheckBase
	secretLister      v1.SecretLister
	runLeaderElection bool
}

var helmSecretsSelector = labels.Set{"owner": "helm"}.AsSelector()

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

	informer := apiClient.InformerFactory.Core().V1().Secrets()
	hc.secretLister = informer.Lister()
	go informer.Informer().Run(stopCh)

	return apiserver.SyncInformers(
		map[apiserver.InformerName]cache.SharedInformer{"helm": informer.Informer()},
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

	secrets, err := hc.secretLister.List(helmSecretsSelector)
	if err != nil {
		return fmt.Errorf("error while listing secrets: %s", err)
	}

	for _, secret := range secrets {
		deployedRelease, err := decodeRelease(string(secret.Data["release"]))
		if err != nil {
			return fmt.Errorf("error while decoding Helm release: %s", err)
		}

		sender.Gauge("helm.release", 1, "", helmTags(deployedRelease))
	}

	return nil
}

func helmTags(release *release) []string {
	return []string{
		fmt.Sprintf("helm_release:%s", release.Name),
		fmt.Sprintf("helm_chart_name:%s", release.Chart.Metadata.Name),
		fmt.Sprintf("helm_namespace:%s", release.Namespace),
		fmt.Sprintf("helm_revision:%d", release.Version),
		fmt.Sprintf("helm_status:%s", release.Info.Status),
		fmt.Sprintf("helm_chart_version:%s", release.Chart.Metadata.Version),
		fmt.Sprintf("helm_app_version:%s", release.Chart.Metadata.AppVersion),
	}
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
