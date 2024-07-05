// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"fmt"
	"regexp"
	"strings"

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

func parseRequestedResource(requestedResource string) (group string, version string, resource string) {
	parts := strings.Split(requestedResource, "/")

	switch len(parts) {
	case 1:
		// format is `{resource}`
		group = ""
		version = ""
		resource = parts[0]
	case 2:
		// format is `{group}/{resource}`
		group = parts[0]
		version = ""
		resource = parts[1]
	case 3:
		// format is `{group}/{version}/{resource}`
		group = parts[0]
		version = parts[1]
		resource = parts[2]
	default:
		// format is not correct
		group = ""
		version = ""
		resource = ""
	}

	return group, version, resource
}

// getGVRsForRequestedResources converts a list of requested resources into a list of GVRs.
//
// If a requested resource doesn't include the api group version, it uses the preferred version discovered
// by the discovery client for the related api group.
//
// Each requested resource should be in the form `{group}/{version}/{resource}`, where {version} is optional.
//
// Items that don't respect this format are skipped
func getGVRsForRequestedResources(discoveryClient discovery.DiscoveryInterface, requestedResource []string) ([]schema.GroupVersionResource, error) {
	groupResourceToVersion, err := discoverGroupResourceVersions(discoveryClient)
	if err != nil {
		return nil, err
	}

	gvrs := make([]schema.GroupVersionResource, 0, len(requestedResource))
	for _, requestedResource := range requestedResource {
		parsedGroup, parsedVersion, parsedResource := parseRequestedResource(requestedResource)

		if parsedVersion != "" {
			// no need to discover preferred version if the version is already known
			gvrs = append(gvrs, schema.GroupVersionResource{
				Resource: parsedResource,
				Group:    parsedGroup,
				Version:  parsedVersion,
			})

			continue
		}

		preferredVersion, found := groupResourceToVersion[schema.GroupResource{Group: parsedGroup, Resource: parsedResource}]
		if found {
			gvrs = append(gvrs, schema.GroupVersionResource{
				Resource: parsedResource,
				Group:    parsedGroup,
				Version:  preferredVersion,
			})
		} else {
			log.Errorf("failed to auto-discover version of group resource %s.%s,", parsedResource, parsedGroup)
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
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return map[schema.GroupResource]string{}, err
		}

		for group, apiGroupErr := range err.(*discovery.ErrGroupDiscoveryFailed).Groups {
			log.Warnf("unable to perform resource discovery for group %s: %s", group, apiGroupErr)
		}
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
