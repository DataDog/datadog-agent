// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// annotationExtractor extracts and transforms a kubernetes objects annotation
// per a callback.
type annotationExtractor[T any] struct {
	key string
	do  func(string) (T, error)
}

// forKey creates a new annotationExtractor for the given key, copying
// the callback function.
func (e annotationExtractor[T]) forKey(key string) annotationExtractor[T] {
	return annotationExtractor[T]{
		key: key,
		do:  e.do,
	}
}

// extract extracts annotation data from the kubernetes Object.
func (e annotationExtractor[T]) extract(o metav1.Object) (T, bool, error) {
	if e.key == "" {
		var empty T
		return empty, false, fmt.Errorf("no key specified for extractor")
	}

	if val, found := o.GetAnnotations()[e.key]; found {
		log.Debugf("Found annotation for %s=%s Single Step Instrumentation.", e.key, val)
		out, err := e.do(val)
		return out, true, err
	}

	var empty T
	return empty, false, nil
}

func infallibleFn[T any](f func(string) T) func(string) (T, error) {
	return func(in string) (T, error) { return f(in), nil }
}
