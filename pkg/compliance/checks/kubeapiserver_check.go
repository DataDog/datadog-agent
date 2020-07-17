// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/jsonquery"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	kubeResourceFieldName      = "kube.resource.name"
	kubeResourceFieldGroup     = "kube.resource.group"
	kubeResourceFieldVersion   = "kube.resource.version"
	kubeResourceFieldNamespace = "kube.resource.namespace"
	kubeResourceFieldKind      = "kube.resource.kind"
	kubeResourceFuncJQ         = "kube.resource.jq"
)

var kubeResourceReportedFields = []string{
	kubeResourceFieldName,
	kubeResourceFieldGroup,
	kubeResourceFieldVersion,
	kubeResourceFieldNamespace,
	kubeResourceFieldKind,
}

func checkKubeapiserver(e env.Env, ruleID string, res compliance.Resource, expr *eval.IterableExpression) (*report, error) {
	if res.KubeApiserver == nil {
		return nil, fmt.Errorf("expecting Kubeapiserver resource in Kubeapiserver check")
	}

	kubeResource := res.KubeApiserver

	if len(kubeResource.Kind) == 0 {
		return nil, fmt.Errorf("cannot run Kubeapiserver check, resource kind is empty")
	}

	if len(kubeResource.APIRequest.Verb) == 0 {
		return nil, fmt.Errorf("cannot run Kubeapiserver check, action verb is empty")
	}

	if len(kubeResource.Version) == 0 {
		kubeResource.Version = "v1"
	}

	resourceSchema := schema.GroupVersionResource{
		Group:    kubeResource.Group,
		Resource: kubeResource.Kind,
		Version:  kubeResource.Version,
	}
	resourceDef := e.KubeClient().Resource(resourceSchema)

	var resourceAPI dynamic.ResourceInterface
	if len(kubeResource.Namespace) > 0 {
		resourceAPI = resourceDef.Namespace(kubeResource.Namespace)
	} else {
		resourceAPI = resourceDef
	}

	var resources []unstructured.Unstructured

	api := kubeResource.APIRequest
	switch api.Verb {
	case "get":
		if len(api.ResourceName) == 0 {
			return nil, fmt.Errorf("unable to use 'get' apirequest without resource name")
		}
		resource, err := resourceAPI.Get(kubeResource.APIRequest.ResourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("unable to get Kube resource:'%v', ns:'%s' name:'%s', err: %v", resourceSchema, kubeResource.Namespace, api.ResourceName, err)
		}
		resources = []unstructured.Unstructured{*resource}
	case "list":

		list, err := resourceAPI.List(metav1.ListOptions{
			LabelSelector: kubeResource.LabelSelector,
			FieldSelector: kubeResource.FieldSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to list Kube resources:'%v', ns:'%s' name:'%s', err: %v", resourceSchema, kubeResource.Namespace, api.ResourceName, err)
		}
		resources = list.Items
	}

	log.Debugf("%s: Got %d resources", ruleID, len(resources))

	it := &kubeResourceIterator{
		resources: resources,
	}

	result, err := expr.EvaluateIterator(it, globalInstance)
	if err != nil {
		return nil, err
	}

	return instanceResultToReport(result, kubeResourceReportedFields), nil
}

type kubeResourceIterator struct {
	resources []unstructured.Unstructured
	index     int
}

func (it *kubeResourceIterator) Next() (*eval.Instance, error) {
	if !it.Done() {
		resource := it.resources[it.index]
		it.index++

		instance := &eval.Instance{
			Vars: eval.VarMap{
				kubeResourceFieldKind:      resource.GetObjectKind().GroupVersionKind().Kind,
				kubeResourceFieldGroup:     resource.GetObjectKind().GroupVersionKind().Group,
				kubeResourceFieldVersion:   resource.GetObjectKind().GroupVersionKind().Version,
				kubeResourceFieldNamespace: resource.GetNamespace(),
				kubeResourceFieldName:      resource.GetName(),
			},
			Functions: eval.FunctionMap{
				kubeResourceFuncJQ: kubeResourceJQ(resource),
			},
		}
		return instance, nil
	}
	return nil, errors.New("out of bounds iteration")
}

func (it *kubeResourceIterator) Done() bool {
	return it.index >= len(it.resources)
}

func kubeResourceJQ(resource unstructured.Unstructured) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
		}
		query, ok := args[0].(string)
		if !ok {
			return nil, errors.New(`expecting string value for query argument"`)
		}

		v, _, err := jsonquery.RunSingleOutput(query, resource.Object)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
}
