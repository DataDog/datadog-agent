// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package k8s

// Secret has no transformer. It is manifest-only (IsMetadataProducer: false): no
// structured metadata model is built or forwarded. Secrets are collected through a
// metadata-only informer, so Data and StringData are never fetched. All processing is
// handled in processors/k8s/secret.go.
