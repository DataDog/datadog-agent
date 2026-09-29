// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeapiserver

import (
	"fmt"
	"iter"
	"slices"
	"sort"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/discovery"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// groupResourceToGVRString is a helper function that converts a group resource string to
// a group-version-resource string
// a group resource string is in the form `{resource}.{group}` or `{resource}` (example: deployments.apps, pods)
// a group version resource string is in the form `{group}/{version}/{resource}` (example: apps/v1/deployments)
// if the groupResource argument is not in the correct format, an empty string is returned
func groupResourceToGVRString(groupResource string) string {
	if len(validation.IsDNS1123Label(groupResource)) == 0 {
		// format is `{resource}`
		return groupResource
	} else if len(validation.IsDNS1123Subdomain(groupResource)) == 0 {
		resource, group, _ := strings.Cut(groupResource, ".")
		// format is `{group}/{version}/{resource}`
		return fmt.Sprintf("%s//%s", group, resource)
	}

	// invalid group resource format
	log.Errorf("invalid group resource %q. must be a valid RFC1123 subdomain in the format `{resource}.{group}` or `{resource}`", groupResource)
	return ""
}

// cleanDuplicateVersions detects if different versions are requested for the same resource within the same group
// it logs an error for each occurrence, and a clean slice that doesn't contain any such duplication
func cleanDuplicateVersions(resources []string) []string {
	groupResourceToVersions := map[schema.GroupResource][]string{}
	cleanedResources := make([]string, 0, len(resources))

	for _, requestedResource := range resources {
		group, version, resourceType := parseRequestedResource(requestedResource)

		versions, found := groupResourceToVersions[schema.GroupResource{Group: group, Resource: resourceType}]
		if found {
			groupResourceToVersions[schema.GroupResource{Group: group, Resource: resourceType}] = append(versions, version)
		} else {
			groupResourceToVersions[schema.GroupResource{Group: group, Resource: resourceType}] = []string{version}
		}
	}

	for gr, versions := range groupResourceToVersions {
		// remove duplicates
		sort.Strings(versions)
		versions = slices.Compact(versions)

		if len(versions) > 1 {
			// there are duplicate versions for the same group/resource
			log.Errorf("can't collect metadata for different versions of the same group and resource: versions requested for %s.%s: %q", gr.Resource, gr.Group, versions)
		} else {
			// only one version requested for the same group/resource
			cleanedResources = append(cleanedResources, fmt.Sprintf("%s/%s/%s", gr.Group, versions[0], gr.Resource))
		}
	}

	return cleanedResources
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
// A group resource is mapped to the preferred version of its group when the resource is served on that version.
// Otherwise, it falls back to a non-preferred version, so that resources served only on a non-preferred version remain discoverable.
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

	// groupResourceToVersion maps a group resource to a discovered version.
	// The preferred version of the group always wins out over others.
	groupResourceToVersion := map[schema.GroupResource]string{}
	for _, resourceList := range apiResourceLists {
		_, isPreferred := preferredGroupVersions[resourceList.GroupVersion]

		// No need to handle error because we are sure it is correctly formatted
		gv, _ := schema.ParseGroupVersion(resourceList.GroupVersion)

		for _, resource := range resourceList.APIResources {
			groupResource := schema.GroupResource{
				Resource: resource.Name,
				Group:    gv.Group,
			}

			// Keep the already-recorded version unless the current one is preferred.
			if _, found := groupResourceToVersion[groupResource]; found && !isPreferred {
				continue
			}

			groupResourceToVersion[groupResource] = gv.Version
		}
	}

	return groupResourceToVersion, nil
}

type entityRelationships struct {
	mu              sync.Mutex
	ownerToChildren map[workloadmeta.EntityID]Set[workloadmeta.EntityID]
}

func newEntityRelationships() *entityRelationships {
	return &entityRelationships{ownerToChildren: make(map[workloadmeta.EntityID]Set[workloadmeta.EntityID])}
}

// addChild records child as a relative of owner.
func (f *entityRelationships) addChild(owner, child workloadmeta.EntityID) {
	f.mu.Lock()
	defer f.mu.Unlock()

	set := f.ownerToChildren[owner]
	set.Add(child)
	f.ownerToChildren[owner] = set
}

// removeChild removes child from owner's ownerToChildren, if present. If owner
// has no remaining children afterwards, its entry is removed from the map.
func (f *entityRelationships) removeChild(owner, child workloadmeta.EntityID) {
	f.mu.Lock()
	defer f.mu.Unlock()

	set, ok := f.ownerToChildren[owner]
	if !ok {
		return
	}
	set.Remove(child)
	if len(set.elements) == 0 {
		delete(f.ownerToChildren, owner)
		return
	}
	f.ownerToChildren[owner] = set
}

// removeOwner deletes owner's entry entirely, regardless of whether it still
// has children. Used when the owner entity itself is removed from workloadmeta.
func (f *entityRelationships) removeOwner(owner workloadmeta.EntityID) {
	f.mu.Lock()
	defer f.mu.Unlock()

	delete(f.ownerToChildren, owner)
}

// children returns a snapshot of owner's ownerToChildren, safe to use without holding the lock.
func (f *entityRelationships) children(owner workloadmeta.EntityID) []workloadmeta.EntityID {
	f.mu.Lock()
	defer f.mu.Unlock()

	set, ok := f.ownerToChildren[owner]
	if !ok {
		return nil
	}

	children := make([]workloadmeta.EntityID, 0, len(set.elements))
	for child := range set.All() {
		children = append(children, child)
	}
	return children
}

type Set[T comparable] struct {
	elements map[T]struct{}
}

func (s *Set[T]) Add(element T) {
	if s.elements == nil {
		s.elements = make(map[T]struct{})
	}
	s.elements[element] = struct{}{}
}

func (s *Set[T]) Contains(element T) bool {
	_, ok := s.elements[element]
	return ok
}

func (s *Set[T]) Remove(element T) {
	if s.Contains(element) {
		delete(s.elements, element)
	}
}

func (s *Set[T]) All() iter.Seq[T] {
	return func(yield func(T) bool) {
		for e := range s.elements {
			if !yield(e) {
				return
			}
		}
	}
}

func (s *Set[T]) Equals(x *Set[T]) bool {
	return false
}
