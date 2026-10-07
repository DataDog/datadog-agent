// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package sbomcollector

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	sbompb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/sbom"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// ---------------------------------------------------------------------------
// fakeStore is a minimal workloadmeta.Component used by the
// workloadmetaEventFromSBOMEventSet tests below. Only GetContainer, GetImage
// and ListImages are exercised by the code under test; the embedded interface
// keeps the type assignable to workloadmeta.Component without forcing us to
// stub the ~30 other methods on the interface.
// ---------------------------------------------------------------------------

type fakeStore struct {
	workloadmeta.Component

	containers map[string]*workloadmeta.Container
	images     map[string]*workloadmeta.ContainerImageMetadata
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		containers: make(map[string]*workloadmeta.Container),
		images:     make(map[string]*workloadmeta.ContainerImageMetadata),
	}
}

func (f *fakeStore) GetContainer(id string) (*workloadmeta.Container, error) {
	if c, ok := f.containers[id]; ok {
		return c, nil
	}
	return nil, errors.New("container not found")
}

func (f *fakeStore) GetImage(id string) (*workloadmeta.ContainerImageMetadata, error) {
	if i, ok := f.images[id]; ok {
		return i, nil
	}
	return nil, errors.New("image not found")
}

func (f *fakeStore) ListImages() []*workloadmeta.ContainerImageMetadata {
	out := make([]*workloadmeta.ContainerImageMetadata, 0, len(f.images))
	for _, i := range f.images {
		out = append(out, i)
	}
	return out
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func component(name, version string, props ...*cyclonedx_v1_4.Property) *cyclonedx_v1_4.Component {
	return &cyclonedx_v1_4.Component{
		Name:       name,
		Version:    version,
		Properties: props,
	}
}

func prop(name, value string) *cyclonedx_v1_4.Property {
	return &cyclonedx_v1_4.Property{Name: name, Value: pointer.Ptr(value)}
}

// findProp returns the value of the first property matching name (or "" if absent).
func findProp(comp *cyclonedx_v1_4.Component, name string) (string, bool) {
	for _, p := range comp.Properties {
		if p != nil && p.Name == name {
			if p.Value == nil {
				return "", true
			}
			return *p.Value, true
		}
	}
	return "", false
}

func TestWorkloadmetaEventFromSBOMEventSet_DoesNotOverridePurl(t *testing.T) {
	// End-to-end check: a system-probe SBOMMessage carrying a different Purl
	// for an already-known component must not change the Purl stored on the
	// image SBOM.
	const containerID = "container-purl"
	const imageID = "image-purl"
	const trustedPurl = "pkg:deb/openssl@1.1.1k?arch=amd64"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, workloadmeta.Success,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			Purl:    pointer.Ptr(trustedPurl),
		},
	)

	msg := systemProbeMessage(t, containerID,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			Purl:    pointer.Ptr("pkg:deb/openssl@1.1.1k?malicious=true"),
			Properties: []*cyclonedx_v1_4.Property{
				prop(sbomutil.LastAccessProperty, "1700000000"),
			},
		},
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)

	img, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	require.NotNil(t, img.SBOM)

	decompressed, err := sbomutil.UncompressSBOM(img.SBOM)
	require.NoError(t, err)
	require.NotNil(t, decompressed.CycloneDXBOM)
	require.Len(t, decompressed.CycloneDXBOM.Components, 1)

	merged := decompressed.CycloneDXBOM.Components[0]
	require.NotNil(t, merged.Purl)
	assert.Equal(t, trustedPurl, *merged.Purl, "system-probe must not be able to overwrite Purl on the image SBOM")

	// Runtime annotation still merged in.
	v, ok := findProp(merged, sbomutil.LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
}

func TestWorkloadmetaEventFromSBOMEventSet_DoesNotAddPurlWhenAbsent(t *testing.T) {
	// End-to-end check: if the image SBOM has no Purl on a component,
	// system-probe must not be able to inject one.
	const containerID = "container-no-purl"
	const imageID = "image-no-purl"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, workloadmeta.Success,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			// Purl intentionally nil.
		},
	)

	msg := systemProbeMessage(t, containerID,
		&cyclonedx_v1_4.Component{
			Name:    "openssl",
			Version: "1.1.1k",
			Purl:    pointer.Ptr("pkg:deb/openssl@1.1.1k?injected=true"),
			Properties: []*cyclonedx_v1_4.Property{
				prop(sbomutil.LastAccessProperty, "1700000000"),
			},
		},
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)

	img, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	require.NotNil(t, img.SBOM)

	decompressed, err := sbomutil.UncompressSBOM(img.SBOM)
	require.NoError(t, err)
	require.Len(t, decompressed.CycloneDXBOM.Components, 1)

	merged := decompressed.CycloneDXBOM.Components[0]
	assert.Nil(t, merged.Purl, "system-probe must not be able to inject a Purl on a component that had none")
}

// ---------------------------------------------------------------------------
// workloadmetaEventFromSBOMEventSet
// ---------------------------------------------------------------------------

// seedImageSBOM constructs a ContainerImageMetadata with a compressed SBOM
// holding the given components and Success status.
func seedImageSBOM(t *testing.T, imageID string, status workloadmeta.SBOMStatus, components ...*cyclonedx_v1_4.Component) *workloadmeta.ContainerImageMetadata {
	t.Helper()

	sbom := &workloadmeta.SBOM{
		CycloneDXBOM: &cyclonedx_v1_4.Bom{
			Components: components,
		},
		Status:             status,
		GenerationTime:     time.Unix(1700000000, 0).UTC(),
		GenerationDuration: 250 * time.Millisecond,
		GenerationMethod:   "tarball",
	}
	compressed, err := sbomutil.CompressSBOM(sbom)
	require.NoError(t, err)

	return &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainerImageMetadata,
			ID:   imageID,
		},
		SBOM: compressed,
	}
}

// systemProbeMessage marshals a CycloneDX BOM into the SBOMMessage shape
// produced by system-probe.
func systemProbeMessage(t *testing.T, containerID string, components ...*cyclonedx_v1_4.Component) *sbompb.SBOMMessage {
	t.Helper()
	data, err := proto.Marshal(&cyclonedx_v1_4.Bom{Components: components})
	require.NoError(t, err)
	return &sbompb.SBOMMessage{
		Kind: string(workloadmeta.KindContainer),
		ID:   containerID,
		Data: data,
	}
}

func TestWorkloadmetaEventFromSBOMEventSet_HappyPath(t *testing.T) {
	const containerID = "container-1"
	const imageID = "image-1"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	existingImage := seedImageSBOM(t, imageID, workloadmeta.Success,
		component("openssl", "1.1.1k", prop("trivy.layer", "sha256:abc")),
	)
	store.images[imageID] = existingImage

	msg := systemProbeMessage(t, containerID,
		component("openssl", "1.1.1k",
			prop(sbomutil.LastAccessProperty, "1700000000"),
			prop(sbomutil.HasSetSuidBitProperty, "true"),
			prop(sbomutil.RunningAsRootProperty, "false"),
		),
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)
	require.Equal(t, workloadmeta.EventTypeSet, event.Type)

	img, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok, "entity should be ContainerImageMetadata, got %T", event.Entity)
	assert.Equal(t, imageID, img.ID)
	require.NotNil(t, img.SBOM)

	// Scan metadata is preserved from the existing image SBOM.
	assert.Equal(t, existingImage.SBOM.Status, img.SBOM.Status)
	assert.Equal(t, existingImage.SBOM.GenerationTime, img.SBOM.GenerationTime)
	assert.Equal(t, existingImage.SBOM.GenerationDuration, img.SBOM.GenerationDuration)
	assert.Equal(t, existingImage.SBOM.GenerationMethod, img.SBOM.GenerationMethod)

	// Decompress and verify runtime properties were merged in.
	decompressed, err := sbomutil.UncompressSBOM(img.SBOM)
	require.NoError(t, err)
	require.NotNil(t, decompressed.CycloneDXBOM)
	require.Len(t, decompressed.CycloneDXBOM.Components, 1)
	merged := decompressed.CycloneDXBOM.Components[0]

	v, ok := findProp(merged, sbomutil.LastAccessProperty)
	assert.True(t, ok)
	assert.Equal(t, "1700000000", v)
	v, ok = findProp(merged, sbomutil.HasSetSuidBitProperty)
	assert.True(t, ok)
	assert.Equal(t, "true", v)
	v, ok = findProp(merged, sbomutil.RunningAsRootProperty)
	assert.True(t, ok)
	assert.Equal(t, "false", v)

	// Original Trivy property is preserved.
	v, ok = findProp(merged, "trivy.layer")
	assert.True(t, ok)
	assert.Equal(t, "sha256:abc", v)
}

func TestWorkloadmetaEventFromSBOMEventSet_FallsBackToRepoDigest(t *testing.T) {
	const containerID = "container-2"
	const repoDigest = "docker.io/foo@sha256:9fb3"
	const configDigest = "sha256:configdigest"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		// Kubelet-style ID that does not match the store key.
		Image: workloadmeta.ContainerImage{ID: repoDigest},
	}

	img := seedImageSBOM(t, configDigest, workloadmeta.Success, component("bash", "5.1"))
	img.RepoDigests = []string{repoDigest}
	store.images[configDigest] = img

	msg := systemProbeMessage(t, containerID,
		component("bash", "5.1", prop(sbomutil.LastAccessProperty, "1700000000")),
	)

	event, err := workloadmetaEventFromSBOMEventSet(store, msg)
	require.NoError(t, err)
	require.Equal(t, workloadmeta.EventTypeSet, event.Type)

	got, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	// The enriched SBOM must be keyed by the resolved image entity's own
	// (config-digest) ID, not the kubelet repo digest. Keying it by the repo
	// digest would create a separate, metadata-less image entity that the SBOM
	// check cannot ship; using the config digest lands it on the same entity the
	// runtime collector populates so the two sources merge and the enriched SBOM
	// is published.
	assert.Equal(t, configDigest, got.ID)
}

func TestWorkloadmetaEventFromSBOMEventSet_PendingSBOMSkipped(t *testing.T) {
	const containerID = "container-3"
	const imageID = "image-3"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, workloadmeta.Pending, component("bash", "5.1"))

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.ErrorIs(t, err, errNotReady)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_MissingExistingSBOM(t *testing.T) {
	const containerID = "container-4"
	const imageID = "image-4"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: imageID},
		// No SBOM.
	}

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.ErrorIs(t, err, errNotReady)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_UnknownImage(t *testing.T) {
	const containerID = "container-5"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: "unknown-image"},
	}

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.ErrorIs(t, err, errNotReady)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_MissingContainer(t *testing.T) {
	store := newFakeStore()

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, "unknown-container"))
	assert.ErrorIs(t, err, errNotReady)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_ContainerWithoutImageID(t *testing.T) {
	const containerID = "container-no-image"

	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		// Image.ID left empty.
	}

	event, err := workloadmetaEventFromSBOMEventSet(store, systemProbeMessage(t, containerID))
	assert.ErrorIs(t, err, errNotReady)
	assert.Nil(t, event.Entity)
}

func TestWorkloadmetaEventFromSBOMEventSet_BadInput(t *testing.T) {
	store := newFakeStore()

	t.Run("nil event", func(t *testing.T) {
		event, err := workloadmetaEventFromSBOMEventSet(store, nil)
		assert.NoError(t, err)
		assert.Nil(t, event.Entity)
	})

	t.Run("wrong kind", func(t *testing.T) {
		msg := &sbompb.SBOMMessage{Kind: "kubernetes_pod", ID: "x"}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		assert.Error(t, err)
		assert.Nil(t, event.Entity)
	})

	t.Run("empty ID", func(t *testing.T) {
		msg := &sbompb.SBOMMessage{Kind: string(workloadmeta.KindContainer)}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		assert.Error(t, err)
		assert.Nil(t, event.Entity)
	})

	t.Run("invalid protobuf data", func(t *testing.T) {
		msg := &sbompb.SBOMMessage{
			Kind: string(workloadmeta.KindContainer),
			ID:   "container-1",
			Data: []byte{0xff, 0xff, 0xff, 0xff},
		}
		event, err := workloadmetaEventFromSBOMEventSet(store, msg)
		assert.Error(t, err)
		assert.Nil(t, event.Entity)
	})
}

// notifyStore records the events notified to it.
type notifyStore struct {
	workloadmeta.Component

	notified []workloadmeta.CollectorEvent
}

func (s *notifyStore) Notify(events []workloadmeta.CollectorEvent) {
	s.notified = append(s.notified, events...)
}

// TestHandleResyncNotifiesEvents checks that the report of the first response
// after a reconnect reaches the store, as the report of any other response does.
func TestHandleResyncNotifiesEvents(t *testing.T) {
	events := []workloadmeta.CollectorEvent{{
		Type:   workloadmeta.EventTypeSet,
		Source: workloadmeta.SourceRemoteSBOMCollector,
		Entity: &workloadmeta.ContainerImageMetadata{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: "sha256:image"},
		},
	}}

	store := &notifyStore{}
	(&streamHandler{}).HandleResync(store, events)

	assert.Equal(t, events, store.notified)
}

// TestForgetRemovedImages checks that the collector unsets its image entity once
// the runtime removes the image, so that a new pull starts from a new scan.
func TestForgetRemovedImages(t *testing.T) {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	forgetRemovedImages(ctx, store)

	// The unset of the collector's entity marks the end of the cleanup.
	image := seedImageSBOM(t, "sha256:abc", workloadmeta.Success, component("bash", "5.1"))
	removed := store.Subscribe("test", workloadmeta.NormalPriority, workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainerImageMetadata).
		SetSource(workloadmeta.SourceRemoteSBOMCollector).
		SetEventType(workloadmeta.EventTypeUnset).
		Build())
	defer store.Unsubscribe(removed)
	unset := make(chan struct{})
	go func() {
		done := unset
		for bundle := range removed {
			bundle.Acknowledge()
			for _, ev := range bundle.Events {
				if done != nil && ev.Entity.GetID() == image.EntityID {
					close(done)
					done = nil
				}
			}
		}
	}()

	store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceRuntime, Entity: image},
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceRemoteSBOMCollector, Entity: image},
	})
	store.Notify([]workloadmeta.CollectorEvent{{
		Type:   workloadmeta.EventTypeUnset,
		Source: workloadmeta.SourceRuntime,
		Entity: &workloadmeta.ContainerImageMetadata{EntityID: image.EntityID},
	}})

	select {
	case <-unset:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the image entity of the collector outlived the image")
	}
	_, err := store.GetImage(image.ID)
	assert.Error(t, err)
}

// readyStore returns a store holding a container of an image whose SBOM is in
// status, with bash 5.1 in it.
func readyStore(t *testing.T, containerID, imageID string, status workloadmeta.SBOMStatus) *fakeStore {
	store := newFakeStore()
	store.containers[containerID] = &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: containerID},
		Image:    workloadmeta.ContainerImage{ID: imageID},
	}
	store.images[imageID] = seedImageSBOM(t, imageID, status, component("bash", "5.1"))
	return store
}

func usedBash(t *testing.T, containerID string) *sbompb.SBOMMessage {
	return systemProbeMessage(t, containerID, component("bash", "5.1", prop(LastAccessProperty, "1700000000")))
}

// lastSeen returns the LastSeenRunning of bash in the image SBOM of event.
func lastSeen(t *testing.T, event workloadmeta.CollectorEvent) string {
	t.Helper()
	image, ok := event.Entity.(*workloadmeta.ContainerImageMetadata)
	require.True(t, ok)
	sbom, err := sbomutil.UncompressSBOM(image.SBOM)
	require.NoError(t, err)
	require.Len(t, sbom.CycloneDXBOM.Components, 1)
	value, _ := findProp(sbom.CycloneDXBOM.Components[0], LastAccessProperty)
	return value
}

// TestHandleResponseKeepsReportsNotReady checks that a report arriving before
// the SBOM of its image waits, and that a merged report replaces it.
func TestHandleResponseKeepsReportsNotReady(t *testing.T) {
	store := readyStore(t, "container", "image", workloadmeta.Pending)
	handler := &streamHandler{pending: newPendingReports()}

	events, err := handler.HandleResponse(store, usedBash(t, "container"))
	require.NoError(t, err)
	assert.Empty(t, events)
	assert.Len(t, handler.pending.list(), 1)

	store.images["image"] = seedImageSBOM(t, "image", workloadmeta.Success, component("bash", "5.1"))
	events, err = handler.HandleResponse(store, usedBash(t, "container"))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Empty(t, handler.pending.list())
}

// TestPendingReportsMerge checks that a pending report merges once the SBOM of
// its image succeeds, and leaves the pending reports.
func TestPendingReportsMerge(t *testing.T) {
	store := readyStore(t, "container", "image", workloadmeta.Pending)
	pending := newPendingReports()
	pending.add(usedBash(t, "container"))

	assert.Empty(t, pending.merge(store))
	assert.Len(t, pending.list(), 1)

	store.images["image"] = seedImageSBOM(t, "image", workloadmeta.Success, component("bash", "5.1"))
	merged := pending.merge(store)
	require.Len(t, merged, 1)
	assert.Equal(t, workloadmeta.SourceRemoteSBOMCollector, merged[0].Source)
	assert.Equal(t, "1700000000", lastSeen(t, merged[0]))
	assert.Empty(t, pending.list())
}

// TestPendingReportsForgetRemovedContainers checks that the report of a
// removed container leaves the pending reports.
func TestPendingReportsForgetRemovedContainers(t *testing.T) {
	pending := newPendingReports()
	pending.add(usedBash(t, "container"))

	pending.forget([]workloadmeta.Event{{
		Type:   workloadmeta.EventTypeUnset,
		Entity: &workloadmeta.Container{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container"}},
	}})
	assert.Empty(t, pending.list())
}

// TestPendingReportsKeepTheLatest checks that a container keeps its latest
// report alone, since each report is a full snapshot of its usage.
func TestPendingReportsKeepTheLatest(t *testing.T) {
	pending := newPendingReports()
	first, latest := usedBash(t, "container"), usedBash(t, "container")
	pending.add(first)
	pending.add(latest)

	reports := pending.list()
	require.Len(t, reports, 1)
	assert.Same(t, latest, reports[0])

	assert.False(t, pending.remove("container", first), "a report a newer one replaced was removed")
	assert.Len(t, pending.list(), 1, "a merge of an older report dropped the latest one")
}

// TestPendingReportsDecodeOnceReady checks that a pending report stays undecoded
// while its image is pending, and that a merge decodes it once the image is ready.
func TestPendingReportsDecodeOnceReady(t *testing.T) {
	store := readyStore(t, "container", "image", workloadmeta.Pending)
	pending := newPendingReports()
	pending.add(&sbompb.SBOMMessage{Kind: string(workloadmeta.KindContainer), ID: "container", Data: []byte{0xff}})

	assert.Empty(t, pending.merge(store))
	assert.Len(t, pending.list(), 1, "the report was decoded before its image was ready")

	store.images["image"] = seedImageSBOM(t, "image", workloadmeta.Success, component("bash", "5.1"))
	assert.Empty(t, pending.merge(store))
	assert.Empty(t, pending.list(), "the undecodable report stayed pending once its image was ready")
}

// TestMergePendingReports checks that a pending report reaches the store once
// the SBOM of its image succeeds there.
func TestMergePendingReports(t *testing.T) {
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	store.Notify([]workloadmeta.CollectorEvent{
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceRuntime, Entity: &workloadmeta.Container{
			EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "container"},
			Image:    workloadmeta.ContainerImage{ID: "image"},
		}},
		{Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceTrivy, Entity: seedImageSBOM(t, "image", workloadmeta.Pending, component("bash", "5.1"))},
	})

	pending := newPendingReports()
	pending.add(usedBash(t, "container"))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mergePendingReports(ctx, store, pending)

	merged := store.Subscribe("test", workloadmeta.NormalPriority, workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainerImageMetadata).
		SetSource(workloadmeta.SourceRemoteSBOMCollector).
		SetEventType(workloadmeta.EventTypeSet).
		Build())
	defer store.Unsubscribe(merged)
	done := make(chan workloadmeta.Event, 1)
	go func() {
		for bundle := range merged {
			bundle.Acknowledge()
			for _, ev := range bundle.Events {
				select {
				case done <- ev:
				default:
				}
			}
		}
	}()

	store.Notify([]workloadmeta.CollectorEvent{{
		Type: workloadmeta.EventTypeSet, Source: workloadmeta.SourceTrivy,
		Entity: seedImageSBOM(t, "image", workloadmeta.Success, component("bash", "5.1")),
	}})

	select {
	case ev := <-done:
		assert.Equal(t, "1700000000", lastSeen(t, workloadmeta.CollectorEvent{Entity: ev.Entity}))
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the pending report never reached the store")
	}
	assert.Empty(t, pending.list())
}
