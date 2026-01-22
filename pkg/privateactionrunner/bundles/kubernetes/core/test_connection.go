// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_core

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type TestConnectionHandler struct {
}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

type TestConnectionInputs struct {
}

type UserInfo struct {
	Username string            `json:"username,omitempty"`
	UID      string            `json:"uid,omitempty"`
	Groups   []string          `json:"groups,omitempty"`
	Extra    map[string]string `json:"extra,omitempty"`
}

type ServerInfo struct {
	Version string `json:"version,omitempty"`
}

type TestConnectionOutputs struct {
	ConfigurationValid bool        `json:"configurationValid"`
	ConnectionValid    bool        `json:"connectionValid"`
	UserInfo           *UserInfo   `json:"userInfo,omitempty"`
	ServerInfo         *ServerInfo `json:"serverInfo,omitempty"`
	Errors             []string    `json:"errors"`
}

func (h *TestConnectionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	var errors []string
	configurationValid := true
	var userInfo *UserInfo
	var serverInfo *ServerInfo

	client, err := kubernetes.KubeClient(credentials)
	if err != nil {
		configurationValid = false
		errors = append(errors, fmt.Sprintf("Configuration validation failed: %v", err))
		return &TestConnectionOutputs{
			ConfigurationValid: configurationValid,
			ConnectionValid:    false,
			UserInfo:           userInfo,
			ServerInfo:         serverInfo,
			Errors:             errors,
		}, nil
	}

	selfSubjectReview := &authv1.SelfSubjectReview{}
	result, err := client.AuthenticationV1().SelfSubjectReviews().Create(ctx, selfSubjectReview, metav1.CreateOptions{})
	if err != nil {
		errors = append(errors, fmt.Sprintf("Failed to connect to Kubernetes cluster: %v", err))
		return &TestConnectionOutputs{
			ConfigurationValid: configurationValid,
			ConnectionValid:    false,
			UserInfo:           userInfo,
			ServerInfo:         serverInfo,
			Errors:             errors,
		}, nil
	}

	extra := make(map[string]string)
	if result.Status.UserInfo.Extra != nil {
		for key, values := range result.Status.UserInfo.Extra {
			if len(values) > 0 {
				extra[key] = values[0]
			}
		}
	}

	userInfo = &UserInfo{
		Username: result.Status.UserInfo.Username,
		UID:      result.Status.UserInfo.UID,
		Groups:   result.Status.UserInfo.Groups,
		Extra:    extra,
	}

	versionInfo, err := client.Discovery().ServerVersion()
	if err != nil {
		errors = append(errors, fmt.Sprintf("Warning: Failed to get server version: %v", err))
	} else {
		serverInfo = &ServerInfo{
			Version: versionInfo.String(),
		}
	}

	return &TestConnectionOutputs{
		ConfigurationValid: configurationValid,
		ConnectionValid:    true,
		UserInfo:           userInfo,
		ServerInfo:         serverInfo,
		Errors:             errors,
	}, nil
}
