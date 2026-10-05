// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ContainerKind identifies a Pod container list.
type ContainerKind string

const (
	// RegularContainers is the application container list.
	RegularContainers ContainerKind = "containers"
	// InitContainers includes ordinary and restartable init containers.
	InitContainers ContainerKind = "initContainers"
)

// ContainerID resolves a container by kind and unique name at write time.
type ContainerID struct {
	Kind ContainerKind
	Name string
}

func (b *batch) container(id ContainerID) ([]string, error) {
	if id.Kind != RegularContainers && id.Kind != InitContainers {
		return nil, errors.New("invalid container kind")
	}
	path := []string{"spec", string(id.Kind)}
	list, err := b.list(path)
	if err != nil {
		return nil, err
	}
	indices, ok := b.containers[id.Kind]
	if !ok {
		indices = make(map[string]int, len(list))
		for i, raw := range list {
			var entry struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &entry) != nil {
				return nil, errors.New("invalid container entry")
			}
			if _, exists := indices[entry.Name]; exists {
				indices[entry.Name] = -1
			} else {
				indices[entry.Name] = i
			}
		}
		b.containers[id.Kind] = indices
	}
	index, exists := indices[id.Name]
	if !exists {
		return nil, errors.New("container not found")
	}
	if index < 0 {
		return nil, errors.New("ambiguous container identity")
	}
	return append(path, strconv.Itoa(index)), nil
}

// ContainerSnapshot returns a detached view of one selected container. It
// avoids decoding and copying the entire Pod for each container-level decision.
func (s *PodSession) ContainerSnapshot(id ContainerID) (*corev1.Container, error) {
	var container corev1.Container
	err := s.batch(func(b *batch) error {
		path, err := b.container(id)
		if err != nil {
			return err
		}
		if view := b.views[pointer(path)]; view != nil {
			container = *view.DeepCopy()
			return nil
		}
		raw, err := b.get(path)
		if err != nil {
			return err
		}
		if s.active != nil {
			prefix := pointer(path)
			var ops []operation
			for _, op := range b.ops {
				if strings.HasPrefix(op.Path, prefix+"/") {
					op.Path = strings.TrimPrefix(op.Path, prefix)
					ops = append(ops, op)
				}
			}
			if len(ops) > 0 {
				wire, err := json.Marshal(ops)
				if err != nil {
					return errors.New("invalid container snapshot operations")
				}
				patch, err := jsonpatch.DecodePatch(wire)
				if err != nil {
					return errors.New("invalid container snapshot patch")
				}
				options := jsonpatch.NewApplyOptions()
				options.SupportNegativeIndices = false
				options.AllowMissingPathOnRemove = false
				options.EnsurePathExistsOnAdd = false
				raw, err = patch.ApplyWithOptions(raw, options)
				if err != nil {
					return errors.New("failed to apply container snapshot operations")
				}
			}
		}
		if json.Unmarshal(raw, &container) != nil {
			return errors.New("invalid container snapshot")
		}
		if s.active != nil {
			b.views[pointer(path)] = container.DeepCopy()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &container, nil
}

// SetAnnotations writes only the specified annotation keys, in sorted order.
func (s *PodSession) SetAnnotations(values map[string]string, ifAbsent bool) error {
	return s.setMap([]string{"metadata", "annotations"}, values, ifAbsent)
}

// SetLabels writes only the specified label keys, in sorted order.
func (s *PodSession) SetLabels(values map[string]string, ifAbsent bool) error {
	return s.setMap([]string{"metadata", "labels"}, values, ifAbsent)
}

// SetNodeSelectors writes only the specified selector keys.
func (s *PodSession) SetNodeSelectors(values map[string]string, ifAbsent bool) error {
	return s.setMap([]string{"spec", "nodeSelector"}, values, ifAbsent)
}

func (s *PodSession) setMap(path []string, values map[string]string, ifAbsent bool) error {
	return s.batch(func(b *batch) error {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			p := append(slices.Clone(path), key)
			raw, err := b.get(p)
			if err != nil {
				return err
			}
			if ifAbsent && len(raw) != 0 {
				continue
			}
			if err := b.set(p, values[key]); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveAnnotations removes selected keys, retaining the parent object.
func (s *PodSession) RemoveAnnotations(keys ...string) error {
	return s.batch(func(b *batch) error {
		for _, key := range keys {
			if err := b.remove([]string{"metadata", "annotations", key}); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetNamespace records an intentional namespace write, never a lookup fallback.
func (s *PodSession) SetNamespace(namespace string) error {
	return s.batch(func(b *batch) error { return b.set([]string{"metadata", "namespace"}, namespace) })
}

// InsertContainer inserts a freshly constructed template, preserving existing entries.
func (s *PodSession) InsertContainer(kind ContainerKind, container corev1.Container, prepend bool) error {
	// Commit earlier field edits before a structural container-list shift, so
	// selected snapshots cannot replay old indices against another occurrence.
	if err := s.flushBatch(); err != nil {
		return err
	}
	if kind != RegularContainers && kind != InitContainers {
		return s.fail(errors.New("invalid container kind"))
	}
	return s.batch(func(b *batch) error { return b.insert([]string{"spec", string(kind)}, container, prepend) })
}

// AppendToleration appends one intentional occurrence without deduplicating others.
func (s *PodSession) AppendToleration(toleration corev1.Toleration) error {
	return s.batch(func(b *batch) error { return b.insert([]string{"spec", "tolerations"}, toleration, false) })
}

// InsertVolume inserts a new volume template.
func (s *PodSession) InsertVolume(volume corev1.Volume, prepend bool) error {
	return s.batch(func(b *batch) error { return b.insert([]string{"spec", "volumes"}, volume, prepend) })
}

// EnvInjection describes a no-overwrite insertion into a selected container.
type EnvInjection struct {
	Container ContainerID
	Env       corev1.EnvVar
	Prepend   bool
}

// EnsureEnvs batches ordered insertions while retaining every existing occurrence.
func (s *PodSession) EnsureEnvs(injections []EnvInjection) (bool, error) {
	changed := false
	err := s.batch(func(b *batch) error {
		for _, injection := range injections {
			path, err := b.container(injection.Container)
			if err != nil {
				return err
			}
			path = append(path, "env")
			list, err := b.list(path)
			if err != nil {
				return err
			}
			found := false
			for _, raw := range list {
				var env corev1.EnvVar
				if err := json.Unmarshal(raw, &env); err != nil {
					return errors.New("invalid environment entry")
				}
				if env.Name == injection.Env.Name {
					found = true
					break
				}
			}
			if found {
				continue
			}
			if err := b.insert(path, injection.Env, injection.Prepend); err != nil {
				return err
			}
			changed = true
		}
		return nil
	})
	return changed && err == nil, err
}

// InsertEnvs inserts fresh occurrences in caller order, including intentional
// duplicate companion variables. Selection and collision policy belong to the caller.
func (s *PodSession) InsertEnvs(injections []EnvInjection) error {
	return s.batch(func(b *batch) error {
		for _, injection := range injections {
			path, err := b.container(injection.Container)
			if err != nil {
				return err
			}
			if err := b.insert(append(path, "env"), injection.Env, injection.Prepend); err != nil {
				return err
			}
		}
		return nil
	})
}

// EnvOccurrence selects one environment entry at the current session revision.
// Any successful intervening write makes it stale and requires a fresh lookup.
type EnvOccurrence struct {
	session   *PodSession
	revision  uint64
	container ContainerID
	index     int
}

// FindEnv returns all matching occurrences, retaining order and identity.
func (s *PodSession) FindEnv(container ContainerID, name string) ([]EnvOccurrence, error) {
	var matches []EnvOccurrence
	err := s.batch(func(b *batch) error {
		path, err := b.container(container)
		if err != nil {
			return err
		}
		list, err := b.list(append(path, "env"))
		if err != nil {
			return err
		}
		for i, raw := range list {
			var env corev1.EnvVar
			if err := json.Unmarshal(raw, &env); err != nil {
				return errors.New("invalid environment entry")
			}
			if env.Name == name {
				matches = append(matches, EnvOccurrence{s, s.revision, container, i})
			}
		}
		return nil
	})
	return matches, err
}

// SetEnvOccurrence edits only the known value fields on the selected occurrence.
// Switching sources explicitly removes the previously selected known subtree.
func (s *PodSession) SetEnvOccurrence(occurrence EnvOccurrence, env corev1.EnvVar) error {
	if occurrence.session != s || occurrence.revision != s.revision {
		return s.fail(errors.New("stale environment occurrence"))
	}
	return s.batch(func(b *batch) error {
		path, err := b.container(occurrence.container)
		if err != nil {
			return err
		}
		path = append(path, "env", strconv.Itoa(occurrence.index))
		if env.ValueFrom == nil {
			if err := b.remove(append(slices.Clone(path), "valueFrom")); err != nil {
				return err
			}
			return b.set(append(path, "value"), env.Value)
		}
		if err := b.remove(append(slices.Clone(path), "value")); err != nil {
			return err
		}
		return b.configureObject(append(path, "valueFrom"), env.ValueFrom, reflect.TypeOf(corev1.EnvVarSource{}))
	})
}

// ResourceEdit sets or removes one resource request or limit.
type ResourceEdit struct {
	Container ContainerID
	Limits    bool
	Name      corev1.ResourceName
	Quantity  *resource.Quantity // nil means intentional removal
}

// EditResources batches recommendations, comparing known quantities semantically.
func (s *PodSession) EditResources(edits []ResourceEdit) error {
	return s.batch(func(b *batch) error {
		for _, edit := range edits {
			path, err := b.container(edit.Container)
			if err != nil {
				return err
			}
			field := "requests"
			if edit.Limits {
				field = "limits"
			}
			path = append(path, "resources", field, string(edit.Name))
			if edit.Quantity == nil {
				if err := b.remove(path); err != nil {
					return err
				}
				continue
			}
			raw, err := b.get(path)
			if err != nil {
				return err
			}
			if len(raw) != 0 {
				var old resource.Quantity
				if json.Unmarshal(raw, &old) == nil && old.Equal(*edit.Quantity) {
					continue
				}
			}
			if err := b.set(path, edit.Quantity); err != nil {
				return err
			}
		}
		return nil
	})
}

// InsertVolumeMount inserts a fresh mount into a uniquely selected container.
func (s *PodSession) InsertVolumeMount(container ContainerID, mount corev1.VolumeMount, prepend bool) error {
	return s.WriteVolumeMounts(nil, []MountEdit{{Container: container, Mount: mount, Prepend: prepend}})
}

// MountEdit describes a selected mount insertion or name+path recipe update.
type MountEdit struct {
	Container ContainerID
	Mount     corev1.VolumeMount
	Prepend   bool
	Configure bool
}

// WriteVolumeMounts batches edits across containers and an optional fresh volume.
// A failure publishes neither the mounts nor the volume.
func (s *PodSession) WriteVolumeMounts(volume *corev1.Volume, edits []MountEdit) error {
	for _, edit := range edits {
		if edit.Configure {
			if err := s.flushContainerField(edit.Container, "volumeMounts"); err != nil {
				return err
			}
		}
	}
	return s.batch(func(b *batch) error {
		for _, edit := range edits {
			if edit.Configure {
				if err := b.configureVolumeMount(edit.Container, edit.Mount, edit.Prepend); err != nil {
					return err
				}
			} else {
				path, err := b.container(edit.Container)
				if err != nil {
					return err
				}
				if err := b.insert(append(path, "volumeMounts"), edit.Mount, edit.Prepend); err != nil {
					return err
				}
			}
		}
		if volume != nil {
			return b.insert([]string{"spec", "volumes"}, *volume, false)
		}
		return nil
	})
}

// RemoveVolumes removes all explicitly selected name occurrences, descending.
func (s *PodSession) RemoveVolumes(names ...string) error {
	return s.batch(func(b *batch) error {
		return b.removeNamed([]string{"spec", "volumes"}, names)
	})
}

// RemoveVolumeMounts removes all selected name occurrences from one container.
func (s *PodSession) RemoveVolumeMounts(container ContainerID, names ...string) error {
	return s.batch(func(b *batch) error {
		path, err := b.container(container)
		if err != nil {
			return err
		}
		return b.removeNamed(append(path, "volumeMounts"), names)
	})
}

func (b *batch) removeNamed(path []string, names []string) error {
	list, err := b.list(path)
	if err != nil {
		return err
	}
	for i := len(list) - 1; i >= 0; i-- {
		var entry struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(list[i], &entry); err != nil {
			return errors.New("invalid named entry")
		}
		if slices.Contains(names, entry.Name) {
			if err := b.removeIndex(path, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// ErrNormalizationConflict indicates that the complete original volume data differs.
// It is a feature-level warning, not a patch engine failure; the batch is untouched.
var ErrNormalizationConflict = errors.New("duplicate volumes differ; cannot safely normalize")

// NormalizeVolumes removes only complete raw duplicates, preserving the first.
func (s *PodSession) NormalizeVolumes() error {
	if err := s.flushBatch(); err != nil {
		return err
	}
	if s.err != nil {
		return s.err
	}
	if s.snapshot != nil && len(s.snapshot.Spec.Volumes) < 2 {
		return nil
	}
	b := s.newBatch()
	path := []string{"spec", "volumes"}
	list, err := b.list(path)
	if err != nil {
		return s.fail(err)
	}
	seen := make(map[string]int, len(list))
	var duplicates []int
	for i, raw := range list {
		var entry struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			return s.fail(errors.New("invalid volume entry"))
		}
		if first, found := seen[entry.Name]; found {
			if !exactEqual(list[first], raw) {
				return ErrNormalizationConflict
			}
			duplicates = append(duplicates, i)
		} else {
			seen[entry.Name] = i
		}
	}
	for i := len(duplicates) - 1; i >= 0; i-- {
		b.ops = append(b.ops, operation{Op: "remove", Path: pointer(append(slices.Clone(path), strconv.Itoa(duplicates[i])))})
	}
	if err := s.apply(b.ops); err != nil {
		return err
	}
	if s.active != nil && len(b.ops) > 0 {
		s.active = s.newBatch()
	}
	return nil
}

// MarkVolumeSafeToEvict preserves the existing comma-list behavior.
func (s *PodSession) MarkVolumeSafeToEvict(annotation, name string) error {
	pod, err := s.Snapshot()
	if err != nil {
		return err
	}
	var volumes []string
	if value := pod.Annotations[annotation]; value != "" {
		volumes = strings.Split(value, ",")
	}
	if slices.Contains(volumes, name) {
		return nil
	}
	return s.SetAnnotations(map[string]string{annotation: strings.Join(append(volumes, name), ",")}, false)
}

// String describes journal size without exposing Pod or patch values.
func (s *PodSession) String() string {
	return fmt.Sprintf("Pod patch session (%d operations)", len(s.journal))
}
