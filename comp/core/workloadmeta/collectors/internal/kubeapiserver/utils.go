// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/runtime/schema"
	utilserror "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/discovery"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func filterMapStringKey(mapInput map[string]string, keyFilters []*regexp.Regexp) map[string]string {
	for key := range mapInput {
		for _, filter := range keyFilters {
			if filter.MatchString(key) {
				delete(mapInput, key)
				// we can break now since the key is already excluded.
				break
			}
		}
	}

	return mapInput
}

func parseFilters(annotationsExclude []string) ([]*regexp.Regexp, error) {
	var parsedFilters []*regexp.Regexp
	var errors []error
	for _, exclude := range annotationsExclude {
		filter, err := filterToRegex(exclude)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		parsedFilters = append(parsedFilters, filter)
	}

	return parsedFilters, utilserror.NewAggregate(errors)
}

// filterToRegex checks a filter's regex
func filterToRegex(filter string) (*regexp.Regexp, error) {
	r, err := regexp.Compile(filter)
	if err != nil {
		errormsg := fmt.Errorf("invalid regex '%s': %s", filter, err)
		return nil, errormsg
	}
	return r, nil
}

func discoverGVRs(discoveryClient discovery.DiscoveryInterface, resources []string) ([]schema.GroupVersionResource, error) {
	discoveredResources, err := discoverResources(discoveryClient)
	if err != nil {
		return nil, err
	}

	gvrs := make([]schema.GroupVersionResource, 0, len(resources))
	for _, resource := range resources {
		gv, found := discoveredResources[resource]
		if found {
			gvrs = append(gvrs, schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: resource})
		} else {
			log.Errorf("failed to auto-discover group/version of resource %s,", resource)
		}
	}

	return gvrs, nil
}

func discoverResources(discoveryClient discovery.DiscoveryInterface) (map[string]schema.GroupVersion, error) {
	apiGroups, apiResourceLists, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}

	preferredGroupVersions := make(map[string]struct{})
	for _, group := range apiGroups {
		preferredGroupVersions[group.PreferredVersion.GroupVersion] = struct{}{}
	}

	discoveredResources := map[string]schema.GroupVersion{}
	for _, resourceList := range apiResourceLists {
		_, found := preferredGroupVersions[resourceList.GroupVersion]
		if found {
			for _, resource := range resourceList.APIResources {
				// No need to handle error because we are sure it is correctly formatted
				gv, _ := schema.ParseGroupVersion(resourceList.GroupVersion)
				discoveredResources[resource.Name] = gv
			}
		}
	}

	return discoveredResources, nil
}
