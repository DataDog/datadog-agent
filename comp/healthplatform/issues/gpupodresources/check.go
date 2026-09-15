// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubelet

package gpupodresources

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
)

const podResourcesRequestTimeout = 5 * time.Second

type checker struct {
	cfg   config.Component
	host  hostnameinterface.Component
	probe func(context.Context) error
}

func newChecker(cfg config.Component, host hostnameinterface.Component) *checker {
	return &checker{
		cfg:  cfg,
		host: host,
		probe: func(ctx context.Context) error {
			client, err := kubelet.NewPodResourcesClient(cfg)
			if err != nil {
				return fmt.Errorf("create PodResources client: %w", err)
			}
			defer client.Close()

			if _, err := client.ListPodResources(ctx); err != nil {
				return fmt.Errorf("list PodResources: %w", err)
			}
			return nil
		},
	}
}

func (c *checker) Run() ([]runnerdef.IssueReport, error) {
	ctx, cancel := context.WithTimeout(context.Background(), podResourcesRequestTimeout)
	defer cancel()

	if err := c.probe(ctx); err != nil {
		return []runnerdef.IssueReport{{
			IssueID:   hostIssueID(c.host.GetSafe(context.Background())),
			IssueName: IssueName,
			Source:    "gpu",
			Context: map[string]string{
				contextKeyError:      err.Error(),
				contextKeySocketPath: c.cfg.GetString("kubernetes_kubelet_podresources_socket"),
			},
		}}, nil
	}
	return nil, nil
}

// hostIssueID scopes the singleton node-local condition to one host. The
// backend deduplicates Agent Health issues by ID across an organization.
func hostIssueID(hostname string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(hostname))
	return fmt.Sprintf("%s:%016x", IssueID, h.Sum64())
}
