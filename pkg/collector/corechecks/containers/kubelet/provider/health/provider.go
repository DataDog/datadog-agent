// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package health

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/kubelet/common"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

// Provider provides the metrics related to data collected from the `/healthz` Kubelet endpoint
type Provider struct {
	config *common.KubeletConfig
}

func NewProvider(config *common.KubeletConfig) *Provider {
	return &Provider{
		config: config,
	}
}

func (p *Provider) Provide(kc kubelet.KubeUtilInterface, sender sender.Sender) error {
	serviceCheckBase := common.KubeletMetricsPrefix + "kubelet.check"
	// Collect raw data
	healthCheckRaw, responseCode, err := kc.QueryKubelet(context.TODO(), "/healthz?verbose")
	if err != nil {
		errMsg := fmt.Sprintf("Kubelet health check failed: %s", err)
		sender.ServiceCheck(serviceCheckBase, servicecheck.ServiceCheckCritical, "", p.config.Tags, errMsg)
		return err
	}

	scanner := bufio.NewScanner(bytes.NewReader(healthCheckRaw))
	re, _ := regexp.Compile(`\[(.)\]([^\s]+) (.*)?`)
	isOk := responseCode == http.StatusOK
	for scanner.Scan() {
		line := scanner.Text()
		result := re.FindStringSubmatch(line)
		// result should have [leftmost matched, status (1st group), name (2nd group)]
		if result == nil || len(result) < 3 {
			continue
		}
		status := result[1]
		serviceCheckName := serviceCheckBase + "." + result[2]
		if status == "+" {
			sender.ServiceCheck(serviceCheckName, servicecheck.ServiceCheckOK, "", p.config.Tags, "")
		} else {
			sender.ServiceCheck(serviceCheckName,
				servicecheck.ServiceCheckCritical,
				"",
				p.config.Tags,
				"")
			isOk = false
		}
	}
	// Report metrics
	if isOk {
		sender.ServiceCheck(serviceCheckBase, servicecheck.ServiceCheckOK, "", p.config.Tags, "")
	} else {
		msg := fmt.Sprintf("Kubelet health check failed, http response code = %d", responseCode)
		sender.ServiceCheck(serviceCheckBase, servicecheck.ServiceCheckCritical, "", p.config.Tags, msg)
	}
	return nil
}
