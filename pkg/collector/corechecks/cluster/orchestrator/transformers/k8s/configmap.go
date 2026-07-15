// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

// ConfigMap has no transformer. It is manifest-only (IsMetadataProducer: false): no
// structured metadata model is built or forwarded. All processing is handled in
// processors/k8s/configmap.go, which strips Data and BinaryData before marshalling.
