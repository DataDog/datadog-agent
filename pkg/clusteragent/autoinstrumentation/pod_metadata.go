package autoinstrumentation

import v1 "k8s.io/api/core/v1"

type PodMetadata struct {
	pod v1.Pod

	initContainers map[string]*v1.Container
	containers     map[string]*v1.Container
	volumes        map[string]*v1.Volume
}

// NewPodMetadata holds information about a pod so se can determine
// langauge and library versions for auto-instrumentation.
func NewPodMetadata(pod v1.Pod) *PodMetadata {
	meta := &PodMetadata{
		pod:            pod,
		initContainers: map[string]*v1.Container{},
		containers:     map[string]*v1.Container{},
		volumes:        map[string]*v1.Volume{},
	}
	meta.init()
	return meta
}

func (d *PodMetadata) init() {
	for _, c := range d.pod.Spec.InitContainers {
		c := c
		d.initContainers[c.Name] = &c
	}
	for _, c := range d.pod.Spec.Containers {
		c := c
		d.containers[c.Name] = &c
	}
	for _, v := range d.pod.Spec.Volumes {
		d.volumes[v.Name] = &v
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
		d.pod.Spec.Volumes = append(d.pod.Spec.Volumes, v)
		d.volumes[v.Name] = &v
	}

	*ref = v
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

type podLibraryConfigState int

const (
	podLibraryConfigStateInit podLibraryConfigState = iota
	podLibraryConfigStateAnnotationsChecked
)

type PodLibraryConfig struct {
	Containers map[string]LibraryConfig

	stage podLibraryConfigState
}

func (p *PodLibraryConfig) SetLanguageInfo(containerName string, l language, info LanguageInfo) {
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
func (d *PodMetadata) PodLibraryConfig(registry string, ls []language) PodLibraryConfig {
	out := PodLibraryConfig{
		Containers: map[string]LibraryConfig{},
		stage:      podLibraryConfigStateInit,
	}

	languageAnnotated := map[language]LanguageInfo{}
	for _, l := range ls {
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
				Image:  l.imageForRegistryAndVersion(registry, version),
				Source: "custom-lib-version-annotation",
			}
		}
	}

	for name, _ := range d.containers {
		for _, l := range ls {
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
					Image:  l.imageForRegistryAndVersion(registry, version),
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

	out.stage = podLibraryConfigStateAnnotationsChecked
	return out
}

func (d *PodMetadata) PodAnnotation(name string) (string, bool) {
	val, found := d.pod.Annotations[name]
	return val, found
}
