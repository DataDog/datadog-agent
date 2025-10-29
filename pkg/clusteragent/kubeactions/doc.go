// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeactions provides functionality for executing Kubernetes actions
// received via remote configuration. Actions are one-time operations like
// deleting pods, restarting deployments, draining nodes, etc.
//
// The package tracks executed actions by metadata ID and version to prevent
// duplicate execution, and provides a framework for validating and executing
// various types of Kubernetes actions.
package kubeactions
