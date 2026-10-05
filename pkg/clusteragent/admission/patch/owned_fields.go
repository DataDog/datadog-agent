// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// SetShareProcessNamespace writes the provider-owned Pod setting.
func (s *PodSession) SetShareProcessNamespace(value bool) error {
	return s.batch(func(b *batch) error { return b.set([]string{"spec", "shareProcessNamespace"}, value) })
}

// EnsureFSGroup writes the two socket-access fields only when FSGroup is absent.
func (s *PodSession) EnsureFSGroup(gid int64) error {
	pod, err := s.Snapshot()
	if err != nil {
		return err
	}
	if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroup != nil {
		return nil
	}
	return s.batch(func(b *batch) error {
		if err := b.set([]string{"spec", "securityContext", "fsGroup"}, gid); err != nil {
			return err
		}
		return b.set([]string{"spec", "securityContext", "fsGroupChangePolicy"}, corev1.FSGroupChangeOnRootMismatch)
	})
}

// EditContainerArg verifies an exact current occurrence before changing it.
func (s *PodSession) EditContainerArg(id ContainerID, index int, expected, value string) error {
	return s.batch(func(b *batch) error {
		path, err := b.container(id)
		if err != nil {
			return err
		}
		path = append(path, "args", strconv.Itoa(index))
		raw, err := b.get(path)
		if err != nil {
			return err
		}
		var current string
		if json.Unmarshal(raw, &current) != nil || current != expected {
			return errors.New("container argument occurrence changed")
		}
		v, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if !exactEqual(raw, v) {
			b.ops = append(b.ops, operation{Op: "replace", Path: pointer(path), Value: v})
			b.values[pointer(path)] = v
			args := path[:len(path)-1]
			list, err := b.list(args)
			if err != nil {
				return err
			}
			list[index] = v
		}
		return nil
	})
}

// AppendContainerArg appends a new scalar argument without rewriting the list.
func (s *PodSession) AppendContainerArg(id ContainerID, value string) error {
	return s.batch(func(b *batch) error {
		path, err := b.container(id)
		if err != nil {
			return err
		}
		return b.insert(append(path, "args"), value, false)
	})
}

// ReplaceVolumeSource explicitly deletes the old known source and installs a
// fresh source, preserving unknown Volume siblings. This deliberately owns the
// deleted source subtree, including any extensions beneath that source.
func (s *PodSession) ReplaceVolumeSource(name string, source corev1.VolumeSource) error {
	return s.batch(func(b *batch) error {
		path := []string{"spec", "volumes"}
		list, err := b.list(path)
		if err != nil {
			return err
		}
		index := -1
		for i, raw := range list {
			var entry corev1.Volume
			if json.Unmarshal(raw, &entry) != nil {
				return errors.New("invalid volume entry")
			}
			if entry.Name == name {
				if index >= 0 {
					return errors.New("ambiguous volume identity")
				}
				index = i
			}
		}
		if index < 0 {
			return errors.New("volume not found")
		}
		path = append(path, strconv.Itoa(index))
		value, err := json.Marshal(source)
		if err != nil {
			return err
		}
		var desired map[string]json.RawMessage
		if err := json.Unmarshal(value, &desired); err != nil {
			return err
		}
		for _, key := range knownJSONFields(reflect.TypeOf(source)) {
			p := append(slices.Clone(path), key)
			current, err := b.get(p)
			if err != nil {
				return err
			}
			v, exists := desired[key]
			if exists && exactEqual(current, v) {
				continue
			}
			if err := b.remove(p); err != nil {
				return err
			}
			if exists {
				if err := b.set(p, v); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// SetMountPath updates the first mount matching its name, as required by CWS.
func (s *PodSession) SetMountPath(id ContainerID, name, mountPath string) error {
	return s.batch(func(b *batch) error {
		path, err := b.container(id)
		if err != nil {
			return err
		}
		path = append(path, "volumeMounts")
		list, err := b.list(path)
		if err != nil {
			return err
		}
		for i, raw := range list {
			var mount corev1.VolumeMount
			if json.Unmarshal(raw, &mount) != nil {
				return errors.New("invalid volume mount")
			}
			if mount.Name == name {
				return b.set(append(path, strconv.Itoa(i), "mountPath"), mountPath)
			}
		}
		return errors.New("volume mount not found")
	})
}

// ConfigureVolumeMount edits the first name+mountPath occurrence, preserving its
// unknown siblings, or inserts a fresh template when none matches.
func (s *PodSession) ConfigureVolumeMount(id ContainerID, mount corev1.VolumeMount, prepend bool) error {
	return s.WriteVolumeMounts(nil, []MountEdit{{Container: id, Mount: mount, Prepend: prepend, Configure: true}})
}

func (b *batch) configureVolumeMount(id ContainerID, mount corev1.VolumeMount, prepend bool) error {
	path, err := b.container(id)
	if err != nil {
		return err
	}
	path = append(path, "volumeMounts")
	list, err := b.list(path)
	if err != nil {
		return err
	}
	for i, raw := range list {
		var existing corev1.VolumeMount
		if json.Unmarshal(raw, &existing) != nil {
			return errors.New("invalid volume mount")
		}
		if existing.Name == mount.Name && existing.MountPath == mount.MountPath {
			return b.configureObject(append(path, strconv.Itoa(i)), mount, reflect.TypeOf(mount))
		}
	}
	return b.insert(path, mount, prepend)
}

// ConfigureSecurityContext sets/removes known fields owned by a template or
// profile override. Unknown fields in surviving nested contexts remain intact.
func (s *PodSession) ConfigureSecurityContext(id ContainerID, desired *corev1.SecurityContext) error {
	return s.batch(func(b *batch) error {
		path, err := b.container(id)
		if err != nil {
			return err
		}
		return b.configureObject(append(path, "securityContext"), desired, reflect.TypeOf(corev1.SecurityContext{}))
	})
}

// ConfigureResources describes a profile's intentional replacement of known
// resource keys, retaining unknown siblings of resources.
func (s *PodSession) ConfigureResources(id ContainerID, desired corev1.ResourceRequirements) error {
	if err := s.flushContainerField(id, "resources"); err != nil {
		return err
	}
	return s.batch(func(b *batch) error { return b.configureResources(id, desired) })
}

func (b *batch) configureResources(id ContainerID, desired corev1.ResourceRequirements) error {
	path, err := b.container(id)
	if err != nil {
		return err
	}
	path = append(path, "resources")
	for _, group := range []struct {
		name   string
		values corev1.ResourceList
	}{{"requests", desired.Requests}, {"limits", desired.Limits}} {
		p := append(slices.Clone(path), group.name)
		raw, err := b.get(p)
		if err != nil {
			return err
		}
		var current map[string]json.RawMessage
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &current); err != nil {
				return errors.New("invalid resource map")
			}
		}
		keys := make([]string, 0, len(current)+len(group.values))
		for key := range current {
			keys = append(keys, key)
		}
		for key := range group.values {
			if _, exists := current[string(key)]; !exists {
				keys = append(keys, string(key))
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			v, exists := group.values[corev1.ResourceName(key)]
			if !exists {
				if err := b.remove(append(slices.Clone(p), key)); err != nil {
					return err
				}
			} else {
				var old corev1.ResourceList
				if len(raw) > 0 && json.Unmarshal(raw, &old) == nil {
					q, exists := old[corev1.ResourceName(key)]
					if exists && q.Equal(v) {
						continue
					}
				}
				if err := b.set(append(slices.Clone(p), key), v); err != nil {
					return err
				}
			}
		}
	}
	// Claims is an explicitly owned known subtree in a resource template.
	if desired.Claims == nil {
		return b.remove(append(path, "claims"))
	}
	return b.set(append(path, "claims"), desired.Claims)
}

// ConfigureInitContainerTemplate installs a fresh injection recipe. For an
// existing init container it edits known fields explicitly, retaining extension
// data on surviving containers, environment occurrences and mounts. Fields and
// entries omitted by the recipe are intentional known-subtree removals.
func (s *PodSession) ConfigureInitContainerTemplate(desired corev1.Container, prepend bool) error {
	pod, err := s.Snapshot()
	if err != nil {
		return err
	}
	found := false
	for _, c := range pod.Spec.InitContainers {
		if c.Name == desired.Name {
			found = true
			break
		}
	}
	if !found {
		return s.InsertContainer(InitContainers, desired, prepend)
	}
	id := ContainerID{InitContainers, desired.Name}
	err = s.batch(func(b *batch) error {
		path, err := b.container(id)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(desired)
		if err != nil {
			return err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		t := reflect.TypeOf(desired)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			key := strings.Split(field.Tag.Get("json"), ",")[0]
			if slices.Contains([]string{"", "-", "name", "env", "volumeMounts", "resources", "securityContext"}, key) {
				continue
			}
			p := append(slices.Clone(path), key)
			value, exists := fields[key]
			if !exists {
				if err := b.remove(p); err != nil {
					return err
				}
				continue
			}
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct && len(value) > 0 && value[0] == '{' {
				if err := b.configureObject(p, value, ft); err != nil {
					return err
				}
			} else if err := b.set(p, value); err != nil {
				return err
			}
		}
		if err := b.configureList(append(slices.Clone(path), "env"), desired.Env, reflect.TypeOf(corev1.EnvVar{}), false); err != nil {
			return err
		}
		if err := b.configureList(append(slices.Clone(path), "volumeMounts"), desired.VolumeMounts, reflect.TypeOf(corev1.VolumeMount{}), true); err != nil {
			return err
		}
		if err := b.configureResources(id, desired.Resources); err != nil {
			return err
		}
		return b.configureObject(append(path, "securityContext"), desired.SecurityContext, reflect.TypeOf(corev1.SecurityContext{}))
	})
	return err
}

func (b *batch) configureList(path []string, desired any, t reflect.Type, mount bool) error {
	raw, err := json.Marshal(desired)
	if err != nil {
		return err
	}
	var templates []json.RawMessage
	if err := json.Unmarshal(raw, &templates); err != nil {
		return err
	}
	existing, err := b.list(path)
	if err != nil {
		return err
	}
	identity := func(raw json.RawMessage) (string, error) {
		var entry struct {
			Name      string `json:"name"`
			MountPath string `json:"mountPath"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return "", errors.New("invalid template entry")
		}
		if mount {
			return entry.Name + "\x00" + entry.MountPath, nil
		}
		return entry.Name, nil
	}
	used := make([]bool, len(existing))
	var additions []json.RawMessage
	for _, template := range templates {
		key, err := identity(template)
		if err != nil {
			return err
		}
		matched := -1
		for i, entry := range existing {
			if used[i] {
				continue
			}
			k, err := identity(entry)
			if err != nil {
				return err
			}
			if k == key {
				matched = i
				break
			}
		}
		if matched < 0 {
			additions = append(additions, template)
			continue
		}
		used[matched] = true
		if err := b.configureObject(append(slices.Clone(path), strconv.Itoa(matched)), template, t); err != nil {
			return err
		}
	}
	for i := len(existing) - 1; i >= 0; i-- {
		if !used[i] {
			if err := b.removeIndex(path, i); err != nil {
				return err
			}
		}
	}
	for _, addition := range additions {
		if err := b.insert(path, addition, false); err != nil {
			return err
		}
	}
	return nil
}

// jsonFields includes Kubernetes inline embedded structs, such as
// SecretKeySelector.LocalObjectReference. Their fields belong to the enclosing object.
func jsonFields(t reflect.Type) []reflect.StructField {
	var fields []reflect.StructField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if f.Anonymous && key == "" && ft.Kind() == reflect.Struct {
			fields = append(fields, jsonFields(ft)...)
		} else if key != "" && key != "-" {
			fields = append(fields, f)
		}
	}
	return fields
}
func knownJSONFields(t reflect.Type) []string {
	var fields []string
	for _, f := range jsonFields(t) {
		fields = append(fields, strings.Split(f.Tag.Get("json"), ",")[0])
	}
	sort.Strings(fields)
	return fields
}

// configureObject is private and only used by explicit owned-field helpers.
// It cannot infer a Pod mutation from two typed objects. A missing known field
// requests deletion of that field, never deletion of its parent object.
func (b *batch) configureObject(path []string, desired any, t reflect.Type) error {
	value, err := json.Marshal(desired)
	if err != nil {
		return errors.New("invalid owned-field value")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(value, &fields); err != nil {
		return errors.New("invalid owned-field object")
	}
	for _, field := range jsonFields(t) {
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		if key == "" || key == "-" {
			continue
		}
		p := append(slices.Clone(path), key)
		v, exists := fields[key]
		if !exists {
			if err := b.remove(p); err != nil {
				return err
			}
			continue
		}
		ft := field.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && len(v) > 0 && v[0] == '{' {
			if err := b.configureObject(p, v, ft); err != nil {
				return err
			}
		} else if err := b.set(p, v); err != nil {
			return err
		}
	}
	return nil
}
