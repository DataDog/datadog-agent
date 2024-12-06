// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// annotationExtractor extracts and transforms a kubernetes objects annotation
// per a callback.
type annotationExtractor[T any] struct {
	key string
	do  func(string) (T, error)
}

var errAnnotationNotFound = errors.New("annotation not found")

func isErrAnnotationNotFound(err error) bool {
	return errors.Is(err, errAnnotationNotFound)
}

// extract extracts annotation data from the kubernetes Object.
func (e annotationExtractor[T]) extract(o metav1.Object) (T, error) {
	if val, found := o.GetAnnotations()[e.key]; found {
		log.Debugf("Found annotation for %s=%s Single Step Instrumentation.", e.key, val)
		out, err := e.do(val)
		return out, err
	}

	var empty T
	return empty, errAnnotationNotFound
}

func infallibleFn[T any](f func(string) T) func(string) (T, error) {
	return func(in string) (T, error) { return f(in), nil }
}
