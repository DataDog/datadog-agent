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

// getGVRsForGroupResources auto discovers groups versions of a list of requested group resources and converts it to a list of GVRs
// each item of requestedGroupResources is of the form {resourceName.apiGroup} (or simply {resourceName} if belonging to the empty api group)
// items that don't respect this format are skipped
func getGVRsForGroupResources(discoveryClient discovery.DiscoveryInterface, groupResources []string) ([]schema.GroupVersionResource, error) {
	groupResourceToVersion, err := discoverGroupResourceVersions(discoveryClient)
	if err != nil {
		return nil, err
	}

	gvrs := make([]schema.GroupVersionResource, 0, len(groupResources))
	for _, groupResourceAsString := range groupResources {
		parsedGroupResource := schema.ParseGroupResource(groupResourceAsString)
		version, found := groupResourceToVersion[parsedGroupResource]
		if found {
			gvrs = append(gvrs, schema.GroupVersionResource{
				Resource: parsedGroupResource.Resource,
				Group:    parsedGroupResource.Group,
				Version:  version,
			})
		} else {
			log.Errorf("failed to auto-discover version of group resource %s,", groupResourceAsString)
		}
	}

	return gvrs, nil
}

// discoverGroupResourceVersions discovers groups, resources, and versions in the kubernetes api server and returns a mapping
// from GroupResource to Version.
// A group resource is mapped to the version that is considered the preferred version by the API Server.
func discoverGroupResourceVersions(discoveryClient discovery.DiscoveryInterface) (map[schema.GroupResource]string, error) {
	apiGroups, apiResourceLists, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}

	preferredGroupVersions := make(map[string]struct{})
	for _, group := range apiGroups {
		preferredGroupVersions[group.PreferredVersion.GroupVersion] = struct{}{}
	}

	// groupResourceToVersion maps a group resource to discovered preferred group version
	groupResourceToVersion := map[schema.GroupResource]string{}
	for _, resourceList := range apiResourceLists {
		_, found := preferredGroupVersions[resourceList.GroupVersion]
		if found {
			for _, resource := range resourceList.APIResources {
				// No need to handle error because we are sure it is correctly formatted
				gv, _ := schema.ParseGroupVersion(resourceList.GroupVersion)

				groupResourceToVersion[schema.GroupResource{
					Resource: resource.Name,
					Group:    gv.Group,
				}] = gv.Version
			}
		}
	}

	return groupResourceToVersion, nil
}
