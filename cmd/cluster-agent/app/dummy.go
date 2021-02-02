// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !kubeapiserver

package app

// this file is to allow go vet to look at something in this directory, as everything
// else currently is excluded (on windows) by build flags
func init() {

}
