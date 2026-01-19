// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"

	support "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UpdateEventHandler struct{}

func NewUpdateEventHandler() *UpdateEventHandler {
	return &UpdateEventHandler{}
}

type UpdateEventInputs struct {
	*support.UpdateFields
	Namespace string    `json:"namespace,omitempty"`
	Body      *v1.Event `json:"body,omitempty"`
}

type UpdateEventOutputs struct {
	ObjectMeta          metav1.ObjectMeta   `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`
	InvolvedObject      v1.ObjectReference  `json:"involvedObject" protobuf:"bytes,2,opt,name=involvedObject"`
	Reason              string              `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
	Message             string              `json:"message,omitempty" protobuf:"bytes,4,opt,name=message"`
	Source              v1.EventSource      `json:"source,omitempty" protobuf:"bytes,5,opt,name=source"`
	FirstTimestamp      metav1.Time         `json:"firstTimestamp,omitempty" protobuf:"bytes,6,opt,name=firstTimestamp"`
	LastTimestamp       metav1.Time         `json:"lastTimestamp,omitempty" protobuf:"bytes,7,opt,name=lastTimestamp"`
	Count               int32               `json:"count,omitempty" protobuf:"varint,8,opt,name=count"`
	Type                string              `json:"type,omitempty" protobuf:"bytes,9,opt,name=type"`
	EventTime           metav1.MicroTime    `json:"eventTime,omitempty" protobuf:"bytes,10,opt,name=eventTime"`
	Series              *v1.EventSeries     `json:"series,omitempty" protobuf:"bytes,11,opt,name=series"`
	Action              string              `json:"action,omitempty" protobuf:"bytes,12,opt,name=action"`
	Related             *v1.ObjectReference `json:"related,omitempty" protobuf:"bytes,13,opt,name=related"`
	ReportingController string              `json:"reportingComponent" protobuf:"bytes,14,opt,name=reportingComponent"`
	ReportingInstance   string              `json:"reportingInstance" protobuf:"bytes,15,opt,name=reportingInstance"`
}

func (h *UpdateEventHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[UpdateEventInputs](task)
	if err != nil {
		return nil, err
	}

	client, err := support.KubeClient(credential)
	if err != nil {
		return nil, err
	}

	response, err := client.CoreV1().Events(inputs.Namespace).Update(ctx, inputs.Body, support.MetaUpdate(inputs.UpdateFields))
	if err != nil {
		return nil, err
	}

	return &UpdateEventOutputs{
		ObjectMeta:          response.ObjectMeta,
		InvolvedObject:      response.InvolvedObject,
		Reason:              response.Reason,
		Message:             response.Message,
		Source:              response.Source,
		FirstTimestamp:      response.FirstTimestamp,
		LastTimestamp:       response.LastTimestamp,
		Count:               response.Count,
		Type:                response.Type,
		EventTime:           response.EventTime,
		Series:              response.Series,
		Action:              response.Action,
		Related:             response.Related,
		ReportingController: response.ReportingController,
		ReportingInstance:   response.ReportingInstance,
	}, nil
}
