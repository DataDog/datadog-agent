// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package webhook

import admiv1beta1 "k8s.io/api/admissionregistration/v1beta1"

// Config contains config parameters
// of the webhook controller
type Config struct {
	name       string
	secretName string
	secretNs   string
	templates  []admiv1beta1.MutatingWebhook
}

// NewConfig creates a webhook controller configuration
func NewConfig(name, secretName, secretNs string, webhooks []admiv1beta1.MutatingWebhook) Config {
	return Config{
		name:       name,
		secretName: secretName,
		secretNs:   secretNs,
		templates:  webhooks,
	}
}

func (w *Config) GetName() string                             { return w.name }
func (w *Config) GetSecretName() string                       { return w.secretName }
func (w *Config) GetSecretNs() string                         { return w.secretNs }
func (w *Config) GetTemplates() []admiv1beta1.MutatingWebhook { return w.templates }
