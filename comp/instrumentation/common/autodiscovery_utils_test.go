// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package common

import (
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

func newCR(name, namespace string, targetKind, targetName string, checks []datadoghq.DatadogInstrumentationCheckConfig) *datadoghq.DatadogInstrumentation {
	return &datadoghq.DatadogInstrumentation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: datadoghq.DatadogInstrumentationSpec{
			TargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: targetKind,
				Name: targetName,
			},
			Config: datadoghq.DatadogInstrumentationConfig{
				Checks: checks,
			},
		},
	}
}

func TestServiceCheckTemplateStore_NotifyOnChange(t *testing.T) {
	store := NewServiceCheckTemplateStore()

	var notified []string
	store.NotifyOnChange(func(namespace, name string) {
		notified = append(notified, namespace+"/"+name)
	})
	// A second subscriber must also be invoked.
	secondCount := 0
	store.NotifyOnChange(func(string, string) {
		secondCount++
	})

	cr := newCR("test", "default", "Service", "svc", nil)

	// Create: service is notified.
	store.WriteTemplates("default/test", cr, []integration.Config{{Name: "check"}})
	require.Equal(t, []string{"default/svc"}, notified)
	assert.Equal(t, 1, secondCount)

	// Config-only update on the same service: single notification (deduplicated).
	store.WriteTemplates("default/test", cr, []integration.Config{{Name: "check2"}})
	require.Equal(t, []string{"default/svc", "default/svc"}, notified)
	assert.Equal(t, 2, secondCount)

	// Delete: service is notified.
	store.DeleteTemplates("default/test")
	require.Equal(t, []string{"default/svc", "default/svc", "default/svc"}, notified)
	assert.Equal(t, 3, secondCount)

	// No-op write (no prior entry, no configs) notifies nothing.
	store.WriteTemplates("default/missing", cr, nil)
	assert.Len(t, notified, 3)
}

func TestCheckStoreHash(t *testing.T) {
	cfg := []integration.Config{{Name: "check"}}
	write := func(cs *CheckStore, key, uid string, gen int64) {
		cr := newCR(key, "ns", "Deployment", "app", nil)
		cr.UID = types.UID(uid)
		cr.Generation = gen
		cs.WriteConfigs(key, cr, cfg)
	}

	t.Run("empty store hashes to zero", func(t *testing.T) {
		require.Equal(t, uint64(0), NewCheckStore().Hash())
	})

	t.Run("add then delete restores empty hash", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		require.NotEqual(t, uint64(0), cs.Hash())
		cs.DeleteConfigs("a")
		require.Equal(t, uint64(0), cs.Hash())
	})

	t.Run("update changes hash and revert restores it", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		write(cs, "b", "uid-b", 1)
		full := cs.Hash()
		write(cs, "b", "uid-b", 2) // generation bump
		require.NotEqual(t, full, cs.Hash())
		write(cs, "b", "uid-b", 1) // revert
		require.Equal(t, full, cs.Hash())
	})

	t.Run("hash is independent of apply order", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		write(cs, "b", "uid-b", 1)

		other := NewCheckStore()
		write(other, "b", "uid-b", 1)
		write(other, "a", "uid-a", 1)

		require.Equal(t, cs.Hash(), other.Hash())
	})

	t.Run("hash is different when same cr has new uid", func(t *testing.T) {
		cs := NewCheckStore()
		write(cs, "a", "uid-a", 1)
		prevHash := cs.Hash()
		write(cs, "a", "uid-b", 1)
		require.NotEqual(t, prevHash, cs.Hash())
	})

	t.Run("checks and logs generation updates do not cancel", func(t *testing.T) {
		// With XOR-combined FNV entry  hashes, this UID produces the same hash
		// for generations 1 and 2 even though both entries have changed.
		const key = "ddi-e2e-update/nginx-monitoring-update"
		const uid = "7a8973da-6e33-5c9e-8150-54b725c7b7d4"
		cs := NewCheckStore()
		write(cs, key, uid, 1)
		write(cs, key+"/logs", uid, 1)
		previousHash := cs.Hash()

		write(cs, key, uid, 2)
		write(cs, key+"/logs", uid, 2)
		require.NotEqual(t, previousHash, cs.Hash())
		configs, hash := cs.ListConfigs()
		require.Len(t, configs, 2)
		require.Equal(t, cs.Hash(), hash)
	})
}
