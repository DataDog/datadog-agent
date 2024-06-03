package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

	Log   log.Component
	WorkloadMeta workloadmeta.Component
}

type provider struct {
	fx.Out

	Component                    api.Component
	PodContainerMetadataEndpoint compapi.AgentEndpointProvider
}

func newProvider(deps dependencies) provider {
	c := &client{
		wmeta: deps.WorkloadMeta,
		log:   deps.Log,
	}

	deps.Lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			c.workloadEvents = c.wmeta.Subscribe(
				"container-inspection",
				workloadmeta.NormalPriority,
				workloadmeta.NewFilter(&workloadmeta.FilterParams{
					EventType: workloadmeta.EventTypeAll,
					Kinds: []workloadmeta.Kind{
						workloadmeta.KindKubernetesPod,
						workloadmeta.KindContainerImageMetadata,
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

	images map[string]*knownWorkload
	pods   map[string]map[string]*workloadmeta.KubernetesPod

	workloadEvents chan workloadmeta.EventBundle
}

type knownWorkload struct {
	image *workloadmeta.ContainerImageMetadata
}

func (c *client) addImageForKey(key string, i *workloadmeta.ContainerImageMetadata) {
	w, ok := c.images[key]
	if !ok {
		w = &knownWorkload{}
	}

	w.image = i
	c.images[key] = w
}

func (c *client) findImageMetadata(i workloadmeta.ContainerImage) (*workloadmeta.ContainerImageMetadata, bool) {
	meta, found := c.images[i.ImageMetadataID()]
	if found {
		return meta.image, true
	}

	if i.RepoDigest != "" {
		meta, found = c.images[i.RepoDigest]
		if found {
			return meta.image, true
		}
	}

	meta, found = c.images[i.RawName]
	if found {
		return meta.image, true
	}

	return nil, false
}

func (c *client) handleEvent(bundle workloadmeta.EventBundle) {
	for _, e := range bundle.Events {
		switch v := e.Entity.(type) {
		case *workloadmeta.ContainerImageMetadata:
			// store image metadata lookups to have candidate lists for
			// images as they come in... We need to be able to look stuff
			// up _super quickly_ for any containers, as we query for them.
			//
			// digests and ids should be 1:1, so that's a pretty cool lookup.
			// but there could be more than one image for a specific _tag_.
			c.addImageForKey(v.ID, v)
			for _, digest := range v.RepoDigests {
				c.addImageForKey(digest, v)
			}
		case *workloadmeta.KubernetesPod:
			// when we get info about a pod...
			// the thing that's important is that we're going to be querying for
			// container data by pod (and container names) later on
			// so that we can start to add some pod data as it comes in.
			//
			// we can store these by namespace and name.
			nsData, ok := c.pods[v.Namespace]
			if !ok {
				nsData = map[string]*workloadmeta.KubernetesPod{}
			}

			nsData[v.Name] = v
			// not sure if we want to process any of the pods here because
			// we might end up processing _all_ of the pods instead of just
			// the ones we need.
			//
			// this is where we might want to put an annotation on the pod
			// that it's being used with injection so that we can do a nice filter
			// on only the ones we care about and get rid of the other ones.
		}
	}
}

func (c *client) findPod(name, namespace string) (*workloadmeta.KubernetesPod, bool) {
	pods, foundNs := c.pods[namespace]
	if !foundNs {
		return nil, false
	}

	pod, found := pods[name]
	return pod, found
}

func (c *client) processPodInitContainers(
	specs map[string]api.ContainerSpec,
	pod *workloadmeta.KubernetesPod,
	out *api.MetadataResponse,
) error {
	for _, container := range pod.InitContainers {
		spec, isRelevantInitContainer := specs[container.Name]
		if !isRelevantInitContainer {
			continue
		}

		if _, alreadyDone := out.Containers[spec.Name]; alreadyDone {
			continue
		}

		image, imageFound := c.findImageMetadata(container.Image)
		if !imageFound {
			return fmt.Errorf("could not find image for container %s", container.Name)
		}

		cmd := determineCmd(spec, image)
		if len(cmd) == 0 {
			// N.B. This might be a "missing info kind of thing" or this might be a fatal error
			// we'll find out when we run out of time.
			return fmt.Errorf("could not determine entry command for container %s", container.Name)
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

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {

		select {
		case <-ctx.Done():
			return out, fmt.Errorf("last error: %w context finished: %w", lastErr, ctx.Err())

		case <-ticker.C:
			pod, podFound := c.findPod(r.PodName, r.PodNamespace)
			if !podFound {
				lastErr = errors.New("could not find pod")
				continue
			}

			err := c.processPodInitContainers(r.InitContainers, pod, &out)
			if err != nil {
				lastErr = err
				continue
			}

			if len(out.Containers) != len(r.InitContainers) {
				lastErr = fmt.Errorf("missing container metadata, expected %v", mapKeys(r.InitContainers))
			}

			return out, nil
		}
	}

	//
	// NOTE(stanistan):
	//
	// This needs to be optimized. With this implementation, every time a pod starts up
	// we are going to look for all the data.
	//
	// In fact, we might want to have this endpoint _take time_ instead
	// of relying on the `inspect` binary (initContainer) to do the retries on its
	// own.
	//
	// This would give us the opportunity to do different levels of searching.
	//
	// 1. We can look for the image by its tag first (RepoTags) if we don't have access to
	//    the pod and store candidate lists for it.
	//
	// 2. We can do some of this work in parallel and _only_ refresh image data once its (1)
	//    stale, or (2) we know that we need to find something else.
	//
	// 3. We might need to add new indexes to workload meta for the things that we are looking for?
	//
	// 4. Older versions of the same containers that _have_ run can give us langauge and process
	//    information.
	//
	// 5. We can add timing to the API request as internal telemetry for container inspection.
	//
	// Basically, to summarize, this is a horrible way of _actually_ doing this, and we should
	// make it better before we go live. _But_ it is fine for the prototype.
	/*
		allImages := c.wmeta.ListImages()
		findImageMetadata := func(name string) *workloadmeta.ContainerImageMetadata {
			for _, i := range allImages {
				if i.ID == name {
					return i
				}
				for _, digest := range i.RepoDigests {
					if digest == name {
						return i
					}
				}
			}
			return nil
		}

		out := MetadataResponse{
			Containers: map[string]ContainerMetadata{},
		}

		for _, c := range pod.InitContainers {
			spec, ok := r.InitContainers[c.Name]
			if !ok {
				continue
			}

			image := findImageMetadata(c.Image.ImageMetadataID())
			if image != nil {
				cmd := spec.determineCmd(image)
				if len(cmd) == 0 {
					return out, fmt.Errorf("could not determine entry command for container %s", c.Name)
				}

				out.Containers[spec.Name] = ContainerMetadata{
					Name:       spec.Name,
					Cmd:        cmd,
					WorkingDir: spec.WorkingDir,
				}
			}

			return out, fmt.Errorf("could not get image for container %s", c.Name)
		}

		if len(r.InitContainers) != len(out.Containers) {
			return out, fmt.Errorf("missing container metadata, try again, expected %v", mapKeys(r.InitContainers))
		}

		return MetadataResponse{}, nil

	*/
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
		q    = r.URL.Query()
		name = q.Get("name")
		ns   = q.Get("ns")
		rb64 = q.Get("request")
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

	mr.PodName = name
	mr.PodNamespace = ns
	mr.InitContainers = containers

	return mr, nil
}

func determineCmd(c api.ContainerSpec, i *workloadmeta.ContainerImageMetadata) []string {
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

func mapKeys[T map[K]V, K comparable, V any](in T) []K {
	keys := make([]K, len(in))

	i := 0
	for k := range in {
		keys[i] = k
		i++
	}

	return keys
}
