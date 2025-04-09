// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package external

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

const (
	watermarkTolerance = 5
)

type recommenderClient struct {
	podWatcher workload.PodWatcher
	client     *http.Client
}

func newRecommenderClient(podWatcher workload.PodWatcher) *recommenderClient {
	return &recommenderClient{
		podWatcher: podWatcher,
		client:     http.DefaultClient,
	}
}

func (r *recommenderClient) GetReplicaRecommendation(ctx context.Context, dpa model.PodAutoscalerInternal) (*model.HorizontalScalingValues, error) {
	recommenderConfig := dpa.CustomRecommenderConfiguration()
	if recommenderConfig == nil { // should not happen; we should not process autoscalers without recommender config
		return nil, fmt.Errorf("external recommender spec is required")
	}

	u, err := url.Parse(recommenderConfig.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("error parsing url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("only http and https schemes are supported")
	}

	req, err := r.buildWorkloadRecommendationRequest(dpa, recommenderConfig)
	if err != nil {
		return nil, err
	}

	payload, err := protojson.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshaling request: %w", err)
	}

	// TODO: We might want to make the timeout configurable later.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client := r.getClient()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "datadog-cluster-agent")
	resp, err := client.Do(httpReq)

	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()

	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	reply := &kubeAutoscaling.WorkloadRecommendationReply{}
	err = protojson.Unmarshal(body, reply)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling response: %w", err)
	}

	return r.buildReplicaRecommendationResponse(reply)
}

// buildWorkloadRecommendationRequest builds a WorkloadRecommendationRequest from DPA and recommender config
func (r *recommenderClient) buildWorkloadRecommendationRequest(dpa model.PodAutoscalerInternal, recommenderConfig *model.RecommenderConfiguration) (*kubeAutoscaling.WorkloadRecommendationRequest, error) {
	objectives := dpa.Spec().Objectives
	if len(objectives) == 0 {
		return nil, fmt.Errorf("no objectives found")
	}
	// Only one objective is supported for now
	objective := objectives[0]

	targetType := ""
	utilization := int32(0)
	switch objective.Type {
	case common.DatadogPodAutoscalerPodResourceObjectiveType:
		targetType = objective.PodResource.Name.String()
		if u := objective.PodResource.Value.Utilization; u != nil {
			utilization = *u
		}
	case common.DatadogPodAutoscalerContainerResourceObjectiveType:
		targetType = objective.ContainerResource.Name.String()
		if u := objective.ContainerResource.Value.Utilization; u != nil {
			utilization = *u
		}
	}

	upperBound, lowerBound := float64(0), float64(0)
	if utilization > 0 {
		upperBound = float64((utilization + watermarkTolerance)) / 100.0
		lowerBound = float64((utilization - watermarkTolerance)) / 100.0
	}

	target := &kubeAutoscaling.WorkloadRecommendationTarget{
		Type:        targetType,
		TargetValue: 0.,
		UpperBound:  upperBound,
		LowerBound:  lowerBound,
	}

	hname, _ := hostname.Get(context.TODO())
	clusterName := clustername.GetRFC1123CompliantClusterName(context.TODO(), hname)

	req := &kubeAutoscaling.WorkloadRecommendationRequest{
		State: &kubeAutoscaling.WorkloadState{
			CurrentReplicas: dpa.CurrentReplicas(),
			ReadyReplicas:   r.getReadyReplicas(dpa),
		},
		TargetRef: &kubeAutoscaling.WorkloadTargetRef{
			Kind:       dpa.Spec().TargetRef.Kind,
			Name:       dpa.Spec().TargetRef.Name,
			ApiVersion: dpa.Spec().TargetRef.APIVersion,
			Namespace:  dpa.Namespace(),
			Cluster:    clusterName,
		},
		Targets: []*kubeAutoscaling.WorkloadRecommendationTarget{target},
	}

	if dpa.Spec().Constraints != nil {
		req.Constraints = &kubeAutoscaling.WorkloadRecommendationConstraints{
			MaxReplicas: dpa.Spec().Constraints.MaxReplicas,
		}
		if dpa.Spec().Constraints.MinReplicas != nil {
			req.Constraints.MinReplicas = *dpa.Spec().Constraints.MinReplicas
		}
	}

	if dpa.ScalingValues().Horizontal != nil {
		req.State.DesiredReplicas = dpa.ScalingValues().Horizontal.Replicas
	}

	if len(recommenderConfig.Settings) > 0 {
		req.Settings = make(map[string]*structpb.Value)
		for k, v := range recommenderConfig.Settings {
			v := v.(string)
			req.Settings[k] = structpb.NewStringValue(v)
		}
	}

	return req, nil
}

// buildReplicaRecommendationResponse builds a ReplicaRecommendationResponse from a WorkloadRecommendationReply
func (r *recommenderClient) buildReplicaRecommendationResponse(reply *kubeAutoscaling.WorkloadRecommendationReply) (*model.HorizontalScalingValues, error) {
	if reply.GetError() != nil {
		err := fmt.Errorf("error from recommender: %d %s", reply.GetError().GetCode(), reply.GetError().GetMessage())
		return nil, err
	}

	ret := &model.HorizontalScalingValues{
		Replicas:  int32(reply.GetTargetReplicas()),
		Timestamp: reply.GetTimestamp().AsTime(),
		Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
	}

	return ret, nil
}

func (r *recommenderClient) getClient() *http.Client {
	if r.client.Transport == nil {
		r.client.Transport = http.DefaultTransport
	}

	// TODO: TLS support
	// if transport, ok := client.Transport.(*http.Transport); ok && tlsConfig != nil {
	// 	tlsTransport, err := NewCertificateReloadingTransport(tlsConfig, r.certificateCache, transport)
	// 	if err != nil {
	// 		return nil, fmt.Errorf("impossible to setup TLS config: %w", err)
	// 	}
	// 	client.Transport = tlsTransport
	// }

	return r.client
}

func (r *recommenderClient) getReadyReplicas(dpa model.PodAutoscalerInternal) *int32 {
	podOwner := workload.NamespacedPodOwner{
		Namespace: dpa.Namespace(),
		Name:      dpa.Spec().TargetRef.Name,
		Kind:      dpa.Spec().TargetRef.Kind,
	}
	pods := r.podWatcher.GetPodsForOwner(podOwner)

	// Count ready pods
	readyReplicas := int32(0)
	for _, pod := range pods {
		if pod.Ready {
			readyReplicas++
		}
	}
	return &readyReplicas
}
