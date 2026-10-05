// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// updateContainerViews updates only detached decision caches for common env
// operations. The raw working document and journal remain authoritative. Any
// other edit invalidates the view, which is rebuilt through the patch library.
// No typed cache is ever serialized into an operation or working document.
func (b *batch) updateContainerViews(ops []operation) {
	for _, op := range ops {
		parts := strings.Split(op.Path, "/")
		if len(parts) < 5 || parts[1] != "spec" || parts[2] != "containers" && parts[2] != "initContainers" {
			continue
		}
		key := strings.Join(parts[:4], "/")
		view := b.views[key]
		if view == nil {
			continue
		}
		if parts[4] != "env" {
			delete(b.views, key)
			continue
		}
		switch len(parts) {
		case 5:
			if op.Op == "remove" {
				view.Env = nil
			} else if json.Unmarshal(op.Value, &view.Env) != nil {
				delete(b.views, key)
			}
		case 6:
			index, err := strconv.Atoi(parts[5])
			if parts[5] == "-" {
				index, err = len(view.Env), nil
			}
			if err != nil || index < 0 || index > len(view.Env) {
				delete(b.views, key)
				continue
			}
			if op.Op == "remove" {
				if index >= len(view.Env) {
					delete(b.views, key)
					continue
				}
				view.Env = slices.Delete(view.Env, index, index+1)
			} else {
				var env corev1.EnvVar
				if json.Unmarshal(op.Value, &env) != nil {
					delete(b.views, key)
					continue
				}
				if op.Op == "add" {
					view.Env = slices.Insert(view.Env, index, env)
				} else if index < len(view.Env) {
					view.Env[index] = env
				} else {
					delete(b.views, key)
				}
			}
		case 7:
			index, err := strconv.Atoi(parts[5])
			if err != nil || index < 0 || index >= len(view.Env) {
				delete(b.views, key)
				continue
			}
			switch parts[6] {
			case "value":
				if op.Op == "remove" {
					view.Env[index].Value = ""
				} else if json.Unmarshal(op.Value, &view.Env[index].Value) != nil {
					delete(b.views, key)
				}
			case "valueFrom":
				if op.Op == "remove" {
					view.Env[index].ValueFrom = nil
				} else if json.Unmarshal(op.Value, &view.Env[index].ValueFrom) != nil {
					delete(b.views, key)
				}
			default:
				delete(b.views, key)
			}
		default:
			delete(b.views, key)
		}
	}
}
