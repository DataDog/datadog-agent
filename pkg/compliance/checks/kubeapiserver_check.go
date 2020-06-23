// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/json"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeDynamic "k8s.io/client-go/dynamic"
)

type kubeApiserverCheck struct {
	baseCheck
	kubeClient   kubeDynamic.Interface
	kubeResource compliance.KubernetesResource
}

const (
	kubeResourceNameKey      string = "kube_resource_name"
	kubeResourceGroupKey     string = "kube_resource_group"
	kubeResourceVersionKey   string = "kube_resource_version"
	kubeResourceNamespaceKey string = "kube_resource_namespace"
	kubeResourceKindKey      string = "kube_resource_kind"
)

func newKubeapiserverCheck(baseCheck baseCheck, kubeResource *compliance.KubernetesResource, kubeClient kubeDynamic.Interface) (*kubeApiserverCheck, error) {
	check := &kubeApiserverCheck{
		baseCheck:    baseCheck,
		kubeClient:   kubeClient,
		kubeResource: *kubeResource,
	}

	if len(check.kubeResource.Kind) == 0 {
		return nil, fmt.Errorf("cannot create Kubeapiserver check, resource kind is empty, rule: %s", baseCheck.ruleID)
	}

	if len(check.kubeResource.APIRequest.Verb) == 0 {
		return nil, fmt.Errorf("cannot create Kubeapiserver check, action verb is empty, rule: %s", baseCheck.ruleID)
	}

	if len(check.kubeResource.Version) == 0 {
		check.kubeResource.Version = "v1"
	}

	return check, nil
}

func (c *kubeApiserverCheck) Run() error {
	log.Debugf("%s: kubeapiserver check: %v", c.ruleID, c.kubeResource)

	resourceSchema := schema.GroupVersionResource{
		Group:    c.kubeResource.Group,
		Resource: c.kubeResource.Kind,
		Version:  c.kubeResource.Version,
	}
	resourceDef := c.kubeClient.Resource(resourceSchema)

	var resourceAPI kubeDynamic.ResourceInterface
	if len(c.kubeResource.Namespace) > 0 {
		resourceAPI = resourceDef.Namespace(c.kubeResource.Namespace)
	} else {
		resourceAPI = resourceDef
	}

	var resources []unstructured.Unstructured
	switch c.kubeResource.APIRequest.Verb {
	case "get":
		if len(c.kubeResource.APIRequest.ResourceName) == 0 {
			return fmt.Errorf("%s: unable to use 'get' apirequest without resource name", c.ruleID)
		}
		resource, err := resourceAPI.Get(c.kubeResource.APIRequest.ResourceName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("unable to get Kube resource:'%v', ns:'%s' name:'%s', err: %v", resourceSchema, c.kubeResource.Namespace, c.kubeResource.APIRequest.ResourceName, err)
		}
		resources = []unstructured.Unstructured{*resource}
	case "list":
		listOptions, err := filterToListOptions(c.kubeResource.Filter)
		if err != nil {
			return fmt.Errorf("%s: unable to parse filters %v, err: %v", c.ruleID, c.kubeResource.Filter, err)
		}

		list, err := resourceAPI.List(listOptions)
		if err != nil {
			return fmt.Errorf("unable to list Kube resources:'%v', ns:'%s' name:'%s', err: %v", resourceSchema, c.kubeResource.Namespace, c.kubeResource.APIRequest.ResourceName, err)
		}
		resources = list.Items
	}

	log.Debugf("%s: Got %d resources", c.ruleID, len(resources))
	for _, resource := range resources {
		report, err := shouldReportResource(c.kubeResource.Filter, resource.Object)
		if err != nil {
			return err
		}

		if !report {
			continue
		}

		if err := c.reportResource(resource); err != nil {
			return err
		}
	}

	return nil
}

func (c *kubeApiserverCheck) reportResource(p unstructured.Unstructured) error {
	kv := compliance.KVMap{}

	for _, field := range c.kubeResource.Report {
		switch field.Kind {
		case compliance.PropertyKindJSONQuery:
			reportValue, valueFound, err := json.RunSingleOutput(field.Property, p.Object)
			if err != nil {
				return fmt.Errorf("unable to report field: '%s' for kubernetes object '%s / %s / %s' - json query error: %v", field.Property, p.GroupVersionKind().String(), p.GetNamespace(), p.GetName(), err)
			}

			if !valueFound {
				continue
			}

			reportName := field.Property
			if len(field.As) > 0 {
				reportName = field.As
			}
			if len(field.Value) > 0 {
				reportValue = field.Value
			}

			kv[reportName] = reportValue
		default:
			return fmt.Errorf("unsupported field kind value: '%s' for kubeApiserver resource", field.Kind)
		}
	}

	if len(kv) > 0 {
		kv[kubeResourceKindKey] = p.GetObjectKind().GroupVersionKind().Kind
		kv[kubeResourceGroupKey] = p.GetObjectKind().GroupVersionKind().Group
		kv[kubeResourceVersionKey] = p.GetObjectKind().GroupVersionKind().Version
		kv[kubeResourceNamespaceKey] = p.GetNamespace()
		kv[kubeResourceNameKey] = p.GetName()
	}

	c.report(nil, kv)
	return nil
}

func shouldReportResource(filters []compliance.Filter, object interface{}) (bool, error) {
	applyCondition := func(c *compliance.Condition) (bool, error) {
		value, _, err := json.RunSingleOutput(c.Property, object)
		if err != nil {
			return false, err
		}

		return evalCondition(value, c), nil
	}

	for _, f := range filters {
		if f.Include != nil {
			if f.Include.Kind == compliance.ConditionKindJSONQuery {
				value, err := applyCondition(f.Include)
				if err != nil {
					return false, err
				}

				if !value {
					return false, nil
				}
			}
		} else if f.Exclude != nil {
			if f.Exclude.Kind == compliance.ConditionKindJSONQuery {
				value, err := applyCondition(f.Exclude)
				if err != nil {
					return false, err
				}

				if value {
					return false, nil
				}
			}
		}
	}

	return true, nil
}

func filterToListOptions(filters []compliance.Filter) (metav1.ListOptions, error) {
	listOptions := metav1.ListOptions{}

	// Exclude it not parsed on purpose as we it makes no sense to have exclude selectors
	labelSelectors := make([]string, 0)
	fieldSelectors := make([]string, 0)
	for _, filter := range filters {
		if filter.Include != nil {
			if filter.Include.Kind == compliance.ConditionKindKubernetesLabelSelector && len(filter.Include.Value) > 0 {
				labelSelectors = append(labelSelectors, filter.Include.Value)
			}

			if filter.Include.Kind == compliance.ConditionKindKubernetesFieldSelector && len(filter.Include.Value) > 0 {
				fieldSelectors = append(fieldSelectors, filter.Include.Value)
			}
		}
	}

	if len(labelSelectors) > 0 {
		listOptions.LabelSelector = strings.Join(labelSelectors, ",")
	}

	if len(fieldSelectors) > 0 {
		listOptions.FieldSelector = strings.Join(fieldSelectors, ",")
	}

	return listOptions, nil
}
