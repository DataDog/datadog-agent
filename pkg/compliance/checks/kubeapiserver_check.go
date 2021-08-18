// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
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

var kubeResourceReportedFields = []string{
	compliance.KubeResourceFieldName,
	compliance.KubeResourceFieldGroup,
	compliance.KubeResourceFieldVersion,
	compliance.KubeResourceFieldNamespace,
	compliance.KubeResourceFieldKind,
}

type kubeUnstructureResolvedResource struct {
	compliance.KubeUnstructuredResource
	eval.Instance
}

func resolveKubeapiserver(ctx context.Context, e env.Env, ruleID string, res compliance.ResourceCommon) (resolved, error) {
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
		resource, err := resourceAPI.Get(ctx, kubeResource.APIRequest.ResourceName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("unable to get Kube resource:'%v', ns:'%s' name:'%s', err: %v", resourceSchema, kubeResource.Namespace, api.ResourceName, err)
		}
		resources = []unstructured.Unstructured{*resource}
	case "list":
		list, err := resourceAPI.List(ctx, metav1.ListOptions{
			LabelSelector: kubeResource.LabelSelector,
			FieldSelector: kubeResource.FieldSelector,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to list Kube resources:'%v', ns:'%s' name:'%s', err: %v", resourceSchema, kubeResource.Namespace, api.ResourceName, err)
		}
		resources = list.Items
	}

	log.Debugf("%s: Got %d resources", ruleID, len(resources))

	instances := make([]resolvedInstance, len(resources))
	for i, resource := range resources {
		instances[i] = &kubeUnstructureResolvedResource{
			KubeUnstructuredResource: compliance.KubeUnstructuredResource{Unstructured: resource},
			Instance: eval.NewInstance(
				eval.VarMap{
					compliance.KubeResourceFieldKind:      resource.GetObjectKind().GroupVersionKind().Kind,
					compliance.KubeResourceFieldGroup:     resource.GetObjectKind().GroupVersionKind().Group,
					compliance.KubeResourceFieldVersion:   resource.GetObjectKind().GroupVersionKind().Version,
					compliance.KubeResourceFieldNamespace: resource.GetNamespace(),
					compliance.KubeResourceFieldName:      resource.GetName(),
				},
				eval.FunctionMap{
					compliance.KubeResourceFuncJQ: kubeResourceJQ(resource),
				},
			),
		}
	}

	return newResolvedInstances(instances), nil
}

func kubeResourceJQ(resource unstructured.Unstructured) eval.Function {
	return func(_ eval.Instance, args ...interface{}) (interface{}, error) {
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
