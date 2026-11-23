// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/containerd/containerd/api/events"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/content"
	containerdevents "github.com/containerd/containerd/v2/core/events"
	"github.com/containerd/containerd/v2/core/images"
	"github.com/gogo/protobuf/types"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadfilterfxmock "github.com/DataDog/datadog-agent/comp/core/workloadfilter/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestKnownImages(t *testing.T) {
	images := newKnownImages()

	imageID := "sha256:92ca512c4cbb8a0909f903979610d5ee300ee26ce2898040c99606d1de46fce1"
	repoTag := "gcr.io/datadoghq/agent:7.42.0"
	repoDigest := "gcr.io/datadoghq/agent@sha256:3a19076bfee70900a600b8e3ee2cc30d5101d1d3d2b33654f1a316e596eaa4e0"

	// Add a reference. The image name is an ID.
	images.addReference(imageID, imageID)
	gotID, found := images.getImageID(imageID)
	assert.True(t, found)
	assert.Equal(t, imageID, gotID)
	assert.Empty(t, images.getRepoTags(imageID))
	assert.Empty(t, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Add another reference to the same image. The name is a repo tag.
	images.addReference(repoTag, imageID)
	gotID, found = images.getImageID(repoTag)
	assert.True(t, found)
	assert.Equal(t, imageID, gotID)
	assert.Equal(t, []string{repoTag}, images.getRepoTags(imageID))
	assert.Empty(t, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Add another reference to the same image. The name is a repo digest.
	images.addReference(repoDigest, imageID)
	gotID, found = images.getImageID(repoDigest)
	assert.True(t, found)
	assert.Equal(t, imageID, gotID)
	assert.Equal(t, []string{repoTag}, images.getRepoTags(imageID))
	assert.Equal(t, []string{repoDigest}, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Delete the reference that is an ID.
	images.deleteReference(imageID, imageID)
	_, found = images.getImageID(imageID)
	assert.False(t, found)
	assert.Equal(t, []string{repoTag}, images.getRepoTags(imageID))
	assert.Equal(t, []string{repoDigest}, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Delete the reference that is a repo tag.
	images.deleteReference(repoTag, imageID)
	_, found = images.getImageID(repoTag)
	assert.False(t, found)
	assert.Empty(t, images.getRepoTags(imageID))
	assert.Equal(t, []string{repoDigest}, images.getRepoDigests(imageID))
	assert.NotEmpty(t, images.getAReference(imageID))

	// Delete the reference that is a repo digest.
	images.deleteReference(repoDigest, imageID)
	_, found = images.getImageID(repoDigest)
	assert.False(t, found)
	assert.Empty(t, images.getRepoTags(imageID))
	assert.Empty(t, images.getRepoDigests(imageID))
	assert.Empty(t, images.getAReference(imageID))
}

func TestGetLayersWithHistory(t *testing.T) {
	// Create a new image
	imageConfig := ocispec.Image{
		History: []ocispec.History{
			{
				EmptyLayer: false,
				Comment:    "not-empty1",
			},
			{
				EmptyLayer: true,
				Comment:    "empty1",
			},
			{
				EmptyLayer: true,
				Comment:    "empty2",
			},
			{
				EmptyLayer: false,
				Comment:    "not-empty2",
			},
			{
				EmptyLayer: true,
				Comment:    "empty3",
			},
			{
				EmptyLayer: false,
				Comment:    "not-empty3",
			},
			{
				EmptyLayer: true,
				Comment:    "empty4",
			},
			{
				EmptyLayer: true,
				Comment:    "empty5",
			},
		},
		RootFS: ocispec.RootFS{
			DiffIDs: []digest.Digest{
				digest.FromString("abc"),
				digest.FromString("def"),
				digest.FromString("ghi"),
				digest.FromString("jkl"),
			},
		},
	}

	manifest := ocispec.Manifest{
		Layers: []ocispec.Descriptor{
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("foo"),
				Size:      1,
			},
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("bar"),
				Size:      2,
			},
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("baz"),
				Size:      3,
			},
			{
				MediaType: ocispec.MediaTypeImageLayer,
				Digest:    digest.FromString("bow"),
				Size:      4,
			},
		},
	}

	layers := getLayersWithHistory(imageConfig, manifest)
	assert.Equal(t, []workloadmeta.ContainerImageLayer{
		{
			MediaType: ocispec.MediaTypeImageLayer,
			DiffID:    digest.FromString("abc").String(),
			SizeBytes: 1,
			History: &ocispec.History{
				Comment:    "not-empty1",
				EmptyLayer: false,
			},
		},
		{
			History: &ocispec.History{
				Comment:    "empty1",
				EmptyLayer: true,
			},
		},
		{
			History: &ocispec.History{
				Comment:    "empty2",
				EmptyLayer: true,
			},
		},
		{
			MediaType: ocispec.MediaTypeImageLayer,
			DiffID:    digest.FromString("def").String(),
			SizeBytes: 2,
			History: &ocispec.History{
				Comment:    "not-empty2",
				EmptyLayer: false,
			},
		},
		{
			History: &ocispec.History{
				Comment:    "empty3",
				EmptyLayer: true,
			},
		},
		{
			MediaType: ocispec.MediaTypeImageLayer,
			DiffID:    digest.FromString("ghi").String(),
			SizeBytes: 3,
			History: &ocispec.History{
				Comment:    "not-empty3",
				EmptyLayer: false,
			},
		},
		{
			History: &ocispec.History{
				Comment:    "empty4",
				EmptyLayer: true,
			},
		},
		{
			History: &ocispec.History{
				Comment:    "empty5",
				EmptyLayer: true,
			},
		},
		{
			MediaType: ocispec.MediaTypeImageLayer,
			DiffID:    digest.FromString("jkl").String(),
			SizeBytes: 4,
		},
	}, layers)
}

// dataContentStore stands for the content store of images whose descriptors
// carry their data, which content.ReadBlob returns without asking the store.
type dataContentStore struct {
	content.Store
}

// blobDescriptor returns the descriptor of data, carrying data.
func blobDescriptor(mediaType string, data []byte) ocispec.Descriptor {
	return ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(data),
		Size:      int64(len(data)),
		Data:      data,
	}
}

// TestImageEventKeepsSBOM checks that the events of an image already scanned
// keep its SBOM, and that an image pulled again after its removal waits for a
// scan.
func TestImageEventKeepsSBOM(t *testing.T) {
	const namespace = "k8s.io"
	ctx := context.Background()

	layer := digest.FromString("layer")
	configBlob, err := json.Marshal(ocispec.Image{
		Platform: ocispec.Platform{OS: "linux", Architecture: "amd64"},
		RootFS:   ocispec.RootFS{Type: "layers", DiffIDs: []digest.Digest{layer}},
	})
	require.NoError(t, err)
	manifestBlob, err := json.Marshal(ocispec.Manifest{
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    blobDescriptor(ocispec.MediaTypeImageConfig, configBlob),
		Layers:    []ocispec.Descriptor{{MediaType: ocispec.MediaTypeImageLayerGzip, Digest: layer, Size: 5}},
	})
	require.NoError(t, err)
	target := blobDescriptor(ocispec.MediaTypeImageManifest, manifestBlob)
	imageID := digest.FromBytes(configBlob).String()

	const repo = "registry.example/app"
	tag, alias, repoDigest := repo+":1", repo+":alias", repo+"@"+target.Digest.String()

	cl, err := containerd.New("", containerd.WithServices(containerd.WithContentStore(dataContentStore{})))
	require.NoError(t, err)
	// names holds the names the runtime gives the image.
	names := map[string]bool{}
	client := &fake.MockedContainerdClient{
		MockImage: func(_, name string) (containerd.Image, error) {
			if !names[name] {
				return nil, fmt.Errorf("no image named %s", name)
			}
			return containerd.NewImage(cl, images.Image{Name: name, Target: target}), nil
		},
		MockListImagesWithDigest: func(_, _ string) ([]containerd.Image, error) {
			var imgs []containerd.Image
			for name := range names {
				imgs = append(imgs, containerd.NewImage(cl, images.Image{Name: name, Target: target}))
			}
			return imgs, nil
		},
	}

	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	provider, err := NewCollector(dependencies{Config: config.NewMock(t), FilterStore: workloadfilterfxmock.SetupMockFilter(t)})
	require.NoError(t, err)
	c := provider.Collector.(*collector)
	c.store = store
	c.containerdClient = client

	notify := func(topic string, msg proto.Message) {
		t.Helper()
		value, err := proto.Marshal(msg)
		require.NoError(t, err)
		require.NoError(t, c.handleImageEvent(ctx, &containerdevents.Envelope{
			Namespace: namespace,
			Topic:     topic,
			Event:     &types.Any{Value: value},
		}))
	}
	pull := func() {
		t.Helper()
		for _, name := range []string{tag, imageID, repoDigest} {
			names[name] = true
			notify(imageCreationTopic, &events.ImageCreate{Name: name})
		}
	}
	image := func() *workloadmeta.ContainerImageMetadata {
		t.Helper()
		img, err := store.GetImage(imageID)
		require.NoError(t, err)
		return img
	}

	pull()
	assert.Equal(t, workloadmeta.Pending, image().SBOM.Status, "SBOM of a new image")

	scanned := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	scan := &workloadmeta.SBOM{
		CycloneDXBOM: &cyclonedx_v1_4.Bom{
			Metadata: &cyclonedx_v1_4.Metadata{
				Component: &cyclonedx_v1_4.Component{
					Name: repo,
					Properties: []*cyclonedx_v1_4.Property{
						{Name: "aquasecurity:trivy:RepoTag", Value: pointer.Ptr(tag)},
						{Name: "aquasecurity:trivy:RepoDigest", Value: pointer.Ptr(repoDigest)},
					},
				},
			},
			Components: []*cyclonedx_v1_4.Component{{Name: "musl", Version: "1.2.5"}},
		},
		Status:         workloadmeta.Success,
		GenerationTime: scanned,
	}
	require.NoError(t, c.handleImageCreateOrUpdate(ctx, namespace, tag, scan))
	contents := func(bom *cyclonedx_v1_4.Bom) []string {
		var s []string
		for _, pkg := range bom.GetComponents() {
			s = append(s, "package "+pkg.GetName()+" "+pkg.GetVersion())
		}
		for _, p := range bom.GetMetadata().GetComponent().GetProperties() {
			s = append(s, "property "+p.GetName()+" "+p.GetValue())
		}
		return s
	}
	// keepsScan checks that the image carries the SBOM of scan, with the repo
	// tags and digests the image has now.
	keepsScan := func(event string) {
		t.Helper()
		img := image()
		assert.Equalf(t, workloadmeta.Success, img.SBOM.Status, "SBOM status after %s", event)
		assert.Truef(t, img.SBOM.GenerationTime.Equal(scanned), "SBOM generated at %s after %s, want %s", img.SBOM.GenerationTime, event, scanned)
		sbom, err := sbomutil.UncompressSBOM(img.SBOM)
		require.NoError(t, err)
		want := sbomutil.UpdateSBOMRepoMetadata(scan, img.RepoTags, img.RepoDigests)
		assert.ElementsMatchf(t, contents(want.CycloneDXBOM), contents(sbom.CycloneDXBOM), "SBOM after %s", event)
	}

	names[alias] = true
	notify(imageCreationTopic, &events.ImageCreate{Name: alias})
	assert.Contains(t, image().RepoTags, alias)
	keepsScan("a new name")

	notify(imageUpdateTopic, &events.ImageUpdate{Name: tag})
	keepsScan("an update")

	delete(names, alias)
	notify(imageDeletionTopic, &events.ImageDelete{Name: alias})
	assert.NotContains(t, image().RepoTags, alias)
	keepsScan("the removal of a name")

	require.NoError(t, c.handleImageCreateOrUpdate(ctx, namespace, tag, &workloadmeta.SBOM{
		Status:         workloadmeta.Failed,
		Error:          "scan not supported",
		GenerationTime: scanned,
	}))
	notify(imageUpdateTopic, &events.ImageUpdate{Name: tag})
	assert.Equal(t, workloadmeta.Failed, image().SBOM.Status, "SBOM status of a failed scan after an update")
	assert.Equal(t, "scan not supported", image().SBOM.Error, "SBOM error of a failed scan after an update")

	for _, name := range []string{tag, repoDigest, imageID} {
		delete(names, name)
		notify(imageDeletionTopic, &events.ImageDelete{Name: name})
	}
	_, err = store.GetImage(imageID)
	require.Error(t, err, "image after the removal of its last name")

	pull()
	assert.Equal(t, workloadmeta.Pending, image().SBOM.Status, "SBOM of an image pulled again")
}
