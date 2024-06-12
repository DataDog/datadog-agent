package autoinstrumentation

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

type PodMetadata struct {
	pod v1.Pod

	initContainers map[string]int
	containers     map[string]int
	volumes        map[string]int
}

// NewPodMetadata holds information about a pod so se can determine
// langauge and library versions for auto-instrumentation.
func NewPodMetadata(pod v1.Pod) *PodMetadata {
	meta := &PodMetadata{
		pod:            pod,
		initContainers: map[string]int{},
		containers:     map[string]int{},
		volumes:        map[string]int{},
	}
	meta.init()
	return meta
}

func (d *PodMetadata) init() {
	for idx, c := range d.pod.Spec.InitContainers {
		d.initContainers[c.Name] = idx
	}
	for idx, c := range d.pod.Spec.Containers {
		d.containers[c.Name] = idx
	}
	for idx, v := range d.pod.Spec.Volumes {
		d.volumes[v.Name] = idx
	}
}

func (d *PodMetadata) ContainsInitContainer(name string) bool {
	_, found := d.initContainers[name]
	return found
}

func (d *PodMetadata) HasVolume(name string) bool {
	_, found := d.volumes[name]
	return found
}

func (d *PodMetadata) WithVolume(v v1.Volume) {
	ref, found := d.volumes[v.Name]
	if !found {
		d.volumes[v.Name] = len(d.pod.Spec.Volumes)
		d.pod.Spec.Volumes = append(d.pod.Spec.Volumes, v)
	} else {
		d.pod.Spec.Volumes[ref] = v
	}
}

func (d *PodMetadata) Get() v1.Pod {
	return d.pod
}

type LanguageInfo struct {
	Image  string
	Source string
}

type LibraryConfig struct {
	Languages map[language]LanguageInfo
}

type PodLibraryConfig struct {
	Containers     map[string]LibraryConfig
	LanguageImages map[language]map[string]struct{}
}

func (p *PodLibraryConfig) SetLanguageInfo(
	containerName string,
	l language,
	info LanguageInfo,
) {
	if p.Containers == nil {
		p.Containers = map[string]LibraryConfig{}
	}

	container, found := p.Containers[containerName]
	if !found {
		container = LibraryConfig{
			Languages: map[language]LanguageInfo{},
		}
	}

	container.Languages[l] = info
	p.Containers[containerName] = container

	images, found := p.LanguageImages[l]
	if !found {
		images = map[string]struct{}{}
	}

	images[info.Image] = struct{}{}
	p.LanguageImages[l] = images
}

type LibConfigOptions struct {
	Registry   string
	Languages  map[language]LanguageInfo
	AutoInject bool
}

// PodLibraryConfig (or whatever this method is going to be called)
// should process the pod and annotations to get us everything
// that we need to know what we're going to inject into
// each container.
//
// Things we might do per container:
// - mount volumes
// - pass configuration to the `inject` container
// - old version would be doing environment var
//   injection as well, but we might some _defer_ stuff for this.
//
// If we know what we should be injecting for the container (and it's non-0
// for the pod we should be able to do a pod mutation.
//
// You should be able to give this the pinned libraries config _after_ the first
// check, and it'll be like "oh is anything set, don't do anything."
func (d *PodMetadata) PodLibraryConfig(opts LibConfigOptions) PodLibraryConfig {
	out := PodLibraryConfig{
		Containers:     map[string]LibraryConfig{},
		LanguageImages: map[language]map[string]struct{}{},
	}

	languageAnnotated := map[language]LanguageInfo{}
	for l, _ := range opts.Languages {
		image, found := d.PodAnnotation(l.customImageAnnotationKey())
		if found {
			languageAnnotated[l] = LanguageInfo{
				Image:  image,
				Source: "custom-image-annotation",
			}
			continue
		}

		version, found := d.PodAnnotation(l.customLibVersionAnnotationKey())
		if found {
			languageAnnotated[l] = LanguageInfo{
				Image:  l.imageForRegistryAndVersion(opts.Registry, version),
				Source: "custom-lib-version-annotation",
			}
		}
	}

	for name, _ := range d.containers {
		for l, _ := range opts.Languages {
			customImage, found := d.PodAnnotation(l.customImageContainerAnnotationKey(name))
			if found {
				out.SetLanguageInfo(name, l, LanguageInfo{
					Image:  customImage,
					Source: "custom-container-image-annotation",
				})
				continue
			}

			version, found := d.PodAnnotation(l.customLibVersionContainerAnnotationKey(name))
			if found {
				out.SetLanguageInfo(name, l, LanguageInfo{
					Image:  l.imageForRegistryAndVersion(opts.Registry, version),
					Source: "custom-container-lib-version-annotation",
				})
				continue
			}

			info, found := languageAnnotated[l]
			if found {
				out.SetLanguageInfo(name, l, info)
				continue
			}
		}
	}

	// if we got nothing we should be doing everything based on the
	// provided configuration and options.
	if len(out.Containers) == 0 && opts.AutoInject {
		for name, _ := range d.containers {
			for l, info := range opts.Languages {
				out.SetLanguageInfo(name, l, info)
			}
		}
	}

	return out
}

func (c *PodLibraryConfig) LanguageInitContainers(
	fn func(*v1.Container),
) ([]v1.Container, error) {
	var (
		cs = make([]v1.Container, len(c.LanguageImages))
		i  = 0
	)
	for l, imageSet := range c.LanguageImages {
		if len(imageSet) != 1 {
			return nil, fmt.Errorf("need exactly one image for language %q, given %d", l, len(imageSet))
		}

		var image string
		for i, _ := range imageSet {
			image = i
			break
		}

		c := v1.Container{
			Name:  l.initContainerName(),
			Image: image,
		}

		fn(&c)
		cs[i] = c
		i++
	}

	return cs, nil
}

func (d *PodMetadata) PodAnnotation(name string) (string, bool) {
	val, found := d.pod.Annotations[name]
	return val, found
}

// AlreadyInjected checks if the pod already has containers injected.
// that's how we know whether we did anything in the first place.
func (d *PodMetadata) AlreadyInjected(opts LibConfigOptions) bool {
	for l, _ := range opts.Languages {
		if _, exists := d.initContainers[l.initContainerName()]; exists {
			return true
		}
	}

	return false
}

func (d *PodMetadata) Mutate(opts LibConfigOptions) (*v1.Pod, error) {
	if d.AlreadyInjected(opts) {
		// already injected, return nil pod
		return nil, nil
	}

	config := d.PodLibraryConfig(opts)

	resources, err := initContainerResourceRequirements()
	if err != nil {
		return nil, err
	}

	langInitContainers, err := config.LanguageInitContainers(func(c *v1.Container) {
		c.Command = []string{"sh", "copy-lib.sh", sharedLibMountPath}
		c.VolumeMounts = []v1.VolumeMount{sharedLibMountPath.mount(sharedLibMountPath, "")}
		c.Resources = resources
	})
	if err != nil {
		return nil, err
	}

	return &d.pod, nil
}
