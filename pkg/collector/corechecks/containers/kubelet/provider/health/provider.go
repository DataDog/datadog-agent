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
	service_check_base := common.KubeletMetricsPrefix + "kubelet.check"
	// Collect raw data
	healthCheckRaw, responseCode, err := kc.QueryKubelet(context.TODO(), "/healthz?verbose")
	if err != nil {
		errMsg := fmt.Sprintf("Kubelet health check failed: %s", err)
		sender.ServiceCheck(service_check_base, servicecheck.ServiceCheckCritical, "", p.config.Tags, errMsg)
		return err
	}

	scanner := bufio.NewScanner(bytes.NewReader(healthCheckRaw))
	re, _ := regexp.Compile(`\[(.)\]([^\s]+) (.*)?`)
	is_ok := responseCode == http.StatusOK
	for scanner.Scan() {
		line := scanner.Text()
		result := re.FindStringSubmatch(line)
		// result should have [leftmost matched, status (1st group), name (2nd group)]
		if result == nil || len(result) < 3 {
			continue
		}
		status := result[1]
		service_check_name := service_check_base + "." + result[2]
		if status == "+" {
			sender.ServiceCheck(service_check_name, servicecheck.ServiceCheckOK, "", p.config.Tags, "")
		} else {
			sender.ServiceCheck(service_check_name,
				servicecheck.ServiceCheckCritical,
				"",
				p.config.Tags,
				"")
			is_ok = false
		}
	}
	// Report metrics
	if is_ok {
		sender.ServiceCheck(service_check_base, servicecheck.ServiceCheckOK, "", p.config.Tags, "")
	} else {
		msg := fmt.Sprintf("Kubelet health check failed, http response code = %d", responseCode)
		sender.ServiceCheck(service_check_base, servicecheck.ServiceCheckCritical, "", p.config.Tags, msg)
	}
	return nil
}
