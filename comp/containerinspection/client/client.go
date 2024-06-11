package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/fx"

	compapi "github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	"github.com/DataDog/datadog-agent/comp/containerinspection/api"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func Module() fxutil.Module {
	return fxutil.Component(fx.Provide(newProvider))
}

type dependencies struct {
	fx.In

	Lc fx.Lifecycle

	Log          log.Component
	WorkloadMeta workloadmeta.Component
}

type provider struct {
	fx.Out

	Component                    api.Component
	PodContainerMetadataEndpoint compapi.AgentEndpointProvider
}

func newProvider(deps dependencies) provider {
	c := &client{
		wmeta:  deps.WorkloadMeta,
		log:    deps.Log,
		images: map[string]*knownWorkload{},
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			c.workloadEvents = c.wmeta.Subscribe(
				"container-inspection",
				workloadmeta.NormalPriority,
				workloadmeta.NewFilter(&workloadmeta.FilterParams{
					EventType: workloadmeta.EventTypeAll,
					Kinds: []workloadmeta.Kind{
						workloadmeta.KindContainerImageMetadata,
						workloadmeta.KindContainer,
					},
				}))

			go func() {
				for ev := range c.workloadEvents {
					ev.Acknowledge()
					c.handleEvent(ev)
				}
			}()
			return nil
		},
		OnStop: func(_ context.Context) error {
			c.wmeta.Unsubscribe(c.workloadEvents)
			return nil
		},
	})

	return provider{
		Component: c,
		PodContainerMetadataEndpoint: compapi.NewAgentEndpointProvider(
			c.PodContainerMetadataHandlerFunc(),
			"/pod-container-metadata",
			"GET",
		),
	}
}

type client struct {
	wmeta workloadmeta.Component
	log   log.Component

	imagesLock sync.RWMutex
	images     map[string]*knownWorkload

	workloadEvents chan workloadmeta.EventBundle
}

func newContainerImageMetadata(i *workloadmeta.ContainerImageMetadata, updatedAt time.Time) containerImageMetadata {
	return containerImageMetadata{
		EntityID:    i.EntityID,
		RepoTags:    i.RepoTags,
		RepoDigests: i.RepoDigests,
		Entrypoint:  i.Entrypoint,
		Cmd:         i.Cmd,
		UpdatedAt:   updatedAt,
	}
}

// containerImageMetadata is a subset of [workloadmeta.ContainerImageMetadata]
// with an update time.
type containerImageMetadata struct {
	EntityID    workloadmeta.EntityID
	RepoTags    []string
	RepoDigests []string
	Entrypoint  []string
	Cmd         []string
	UpdatedAt   time.Time
}

type knownWorkload struct {
	image *containerImageMetadata
}

func (c *client) addImageForKey(key string, i *containerImageMetadata) {
	w, ok := c.images[key]
	if !ok {
		w = &knownWorkload{}
	}

	w.image = i
	c.images[key] = w
}

func (c *client) findImageMetadata(i workloadmeta.ContainerImage) (*containerImageMetadata, bool) {
	c.imagesLock.RLock()
	defer c.imagesLock.RUnlock()

	for _, lookupKey := range []string{
		i.ImageMetadataID(), // we first check by the id to see if we can find it
		i.RepoDigest,        // then by digest if present
		i.RawName,           // then raw name
	} {
		if lookupKey == "" {
			continue
		}

		meta, found := c.images[lookupKey]
		if found {
			return meta.image, true
		}
	}

	return nil, false
}

func (c *client) findImageByName(name string) (*containerImageMetadata, bool) {
	c.imagesLock.RLock()
	defer c.imagesLock.RUnlock()

	meta, found := c.images[name]
	if found {
		return meta.image, found
	}

	return nil, false
}

func (c *client) addContainer(i *workloadmeta.Container) {
	c.log.Debugf("workloadmeta.Container{ Meta: %+v, Image: %+v }", i.EntityMeta, i.Image)

}

func (c *client) addImageMetadata(i *workloadmeta.ContainerImageMetadata, t time.Time) {
	c.imagesLock.Lock()
	defer c.imagesLock.Unlock()
	// store image metadata lookups to have candidate lists for
	// images as they come in... We need to be able to look stuff
	// up _super quickly_ for any containers, as we query for them.
	//
	// digests and ids should be 1:1, so that's a pretty cool lookup.
	// but there could be more than one image for a specific _tag_.
	image := newContainerImageMetadata(i, t)
	c.log.Debugf("adding image metadata %+v", image)
	c.addImageForKey(i.ID, &image)
	for _, digest := range i.RepoDigests {
		c.addImageForKey(digest, &image)
	}
	for _, tag := range i.RepoTags {
		c.addImageForKey(tag, &image)
	}
}

func (c *client) handleEvent(bundle workloadmeta.EventBundle) {
	for _, e := range bundle.Events {
		switch v := e.Entity.(type) {
		case *workloadmeta.Container:
			if e.Type == workloadmeta.EventTypeSet {
				c.addContainer(v)
			}
		case *workloadmeta.ContainerImageMetadata:
			if e.Type == workloadmeta.EventTypeSet {
				c.addImageMetadata(v, time.Now())
			}
		}
	}
}

func (c *client) processContainersForSpec(
	containerSpecs map[string]api.ContainerSpec,
	out *api.MetadataResponse,
	staleImageTime time.Time,
) error {
	for _, spec := range containerSpecs {
		if _, alreadyDone := out.Containers[spec.Name]; alreadyDone {
			continue
		}

		image, imageFound := c.findImageByName(spec.Image)
		if !imageFound {
			return fmt.Errorf("could not find image for container %s", spec.Image)
		}

		// There's a scenario where we have an _old_ image here and haven't processed
		// the new data in the container...
		if image.UpdatedAt.Before(staleImageTime) {
			return fmt.Errorf("found image but it might be old %v", image)
		}

		cmd := determineCmd(spec, image)
		if len(cmd) == 0 {
			// N.B. This might be a "missing info kind of thing" or this might be a fatal error
			// we'll find out when we run out of time.
			return fmt.Errorf("could not determine entry command for container %s", spec.Name)
		}

		out.Containers[spec.Name] = api.ContainerMetadata{
			Name:       spec.Name,
			Cmd:        cmd,
			WorkingDir: spec.WorkingDir,
		}
	}

	return nil
}

func (c *client) PodContainerMetadata(ctx context.Context, r api.MetadataRequest) (api.MetadataResponse, error) {
	var (
		out     = api.MetadataResponse{Containers: map[string]api.ContainerMetadata{}}
		lastErr error
	)

	c.log.Debugf("request -> %+v", r)

	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	staleImageTime := time.Unix(0, 0)
	if r.StaleImageDuration != nil {
		staleImageTime = staleImageTime.Add(*r.StaleImageDuration * -1)
	}

	for {

		select {
		case <-ctx.Done():
			c.log.Debugf("deadline exceeded: %v", lastErr)
			return out, fmt.Errorf("last error: %w context finished: %w", lastErr, ctx.Err())

		case <-ticker.C:
			err := c.processContainersForSpec(r.InitContainers, &out, staleImageTime)
			if err != nil {
				c.log.Debugf("got error processing spec = %v", err)
				lastErr = err
				continue
			}

			return out, nil
		}
	}
}

func (c *client) PodContainerMetadataHandlerFunc() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		output, err, code := c.podContainerMetadataForHTTPRequest(r)
		if err != nil {
			utils.SetJSONError(w, err, code)
			return
		}

		jsonDump, err := json.Marshal(output)
		if err != nil {
			utils.SetJSONError(w, c.log.Errorf("unable to marshal pod container metadata: %v", err), 500)
			return
		}

		_, _ = w.Write(jsonDump)
	}
}

func (c *client) podContainerMetadataForHTTPRequest(r *http.Request) (api.MetadataResponse, error, int) {
	mr, err := metadataRequestFromHTTPRequest(r)
	if err != nil {
		return api.MetadataResponse{}, fmt.Errorf("malformed request: %w", err), 400
	}

	out, err := c.PodContainerMetadata(r.Context(), mr)
	if err != nil {
		return api.MetadataResponse{}, fmt.Errorf("could not get metadata: %w", err), 500
	}

	return out, nil, 200
}

func metadataRequestFromHTTPRequest(r *http.Request) (api.MetadataRequest, error) {
	var (
		q        = r.URL.Query()
		name     = q.Get("name")
		ns       = q.Get("ns")
		rb64     = q.Get("request")
		duration = q.Get("duration")
	)

	var mr api.MetadataRequest
	if name == "" {
		return mr, errors.New("missing name")
	}

	if ns == "" {
		return mr, errors.New("missing ns")
	}

	if rb64 == "" {
		return mr, errors.New("missing base64 encoded request payload")
	}

	containers, err := decodeContainers(rb64)
	if err != nil {
		return mr, err
	}

	if duration == "" {
		duration = "5m" // default duration
	}

	staleImageDuration, err := time.ParseDuration(duration)
	if err != nil {
		return mr, fmt.Errorf("invalid stale image duration: %w", err)
	}

	mr.PodName = name
	mr.PodNamespace = ns
	mr.InitContainers = containers
	mr.StaleImageDuration = &staleImageDuration

	return mr, nil
}

func determineCmd(c api.ContainerSpec, i *containerImageMetadata) []string {
	var out []string
	if len(c.Command) != 0 {
		out = c.Command
	} else if len(i.Entrypoint) > 0 {
		out = i.Entrypoint
	}

	if len(c.Args) > 0 {
		out = append(out, c.Args...)
	} else if len(i.Cmd) > 0 {
		out = append(out, i.Cmd...)
	}

	return out
}

func decodeContainers(data string) (map[string]api.ContainerSpec, error) {
	bs, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}

	var containers map[string]api.ContainerSpec
	err = json.Unmarshal(bs, &containers)
	if err != nil {
		return nil, err
	}

	return containers, nil
}
