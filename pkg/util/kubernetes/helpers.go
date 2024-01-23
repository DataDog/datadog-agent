// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

// KubeAllowedEncodeStringAlphaNums holds the charactes allowed in replicaset names from as parent deployment
// Taken from https://github.com/kow3ns/kubernetes/blob/96067e6d7b24a05a6a68a0d94db622957448b5ab/staging/src/k8s.io/apimachinery/pkg/util/rand/rand.go#L76
const KubeAllowedEncodeStringAlphaNums = "bcdfghjklmnpqrstvwxz2456789"

// Digits holds the digits used for naming replicasets in kubenetes < 1.8
const Digits = "1234567890"

// ParseDeploymentForReplicaSet gets the deployment name from a replicaset,
// or returns an empty string if no parent deployment is found.
func ParseDeploymentForReplicaSet(name string) string {
	panic("not called")
}

// ParseCronJobForJob gets the cronjob name from a job,
// or returns an empty string if no parent cronjob is found.
// https://github.com/kubernetes/kubernetes/blob/b4e3bd381bd4d7c0db1959341b39558b45187345/pkg/controller/cronjob/utils.go#L156
func ParseCronJobForJob(name string) (string, int) {
	panic("not called")
}
