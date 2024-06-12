package autoinstrumentation

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "k8s.io/api/core/v1"
)

func TestParsingPodMetadata(t *testing.T) {
	pod := &v1.Pod{}

	meta := NewPodMetadata(*pod)
	require.NotNil(t, meta)
	require.False(t, meta.ContainsInitContainer("anything"))

	_, exists := meta.PodAnnotation("foo")
	require.False(t, exists)

	pod.Annotations = map[string]string{"foo": "bar"}
	_, exists = meta.PodAnnotation("foo")
	require.Falsef(t, exists, "we should not be able to mutate the pod")

	volume := v1.Volume{
		Name: "test-volume",
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	require.False(t, meta.HasVolume(volume.Name))
	meta.WithVolume(volume)

	require.True(t, meta.HasVolume(volume.Name))
	require.Equal(t, volume, meta.pod.Spec.Volumes[0], "using WithVolume sets the pod")

	volume2 := v1.Volume{
		Name: "test-volume",
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "test",
			},
		},
	}
	meta.WithVolume(volume2)
	require.Equal(t, 1, len(meta.pod.Spec.Volumes))
	require.Equal(t, volume2, meta.pod.Spec.Volumes[0], "updating a volume mutates it")
}

func TestLanguageAndLibDetection(t *testing.T) {
	var (
		registry         = "registry"
		podLibraryConfig = func(pod *v1.Pod) PodLibraryConfig {
			return NewPodMetadata(*pod).PodLibraryConfig(LibConfigOptions{
				Registry: registry,
				Languages: map[language]LanguageInfo{
					js:   js.defaultLanguageInfo(registry),
					java: java.defaultLanguageInfo(registry),
				},
				AutoInject: true,
			})
		}
		requireContainersConfig = func(t *testing.T, c map[string]LibraryConfig, pod *v1.Pod, message string, args ...interface{}) {
			t.Helper()
			require.Equalf(t, c, podLibraryConfig(pod).Containers, message, args...)
		}
		requireLanguagesConfig = func(t *testing.T, c map[language]map[string]struct{}, pod *v1.Pod, message string, args ...interface{}) {
			t.Helper()
			require.Equalf(t, c, podLibraryConfig(pod).LanguageImages, message, args...)
		}
	)

	requireContainersConfig(t,
		map[string]LibraryConfig{
			"applies-here": {
				Languages: map[language]LanguageInfo{
					js: {
						Image:  "registry/dd-lib-js-init:my-custom-val",
						Source: "custom-lib-version-annotation",
					},
				},
			},
			"applies-here-too": {
				Languages: map[language]LanguageInfo{
					js: {
						Image:  "registry/dd-lib-js-init:my-custom-val",
						Source: "custom-lib-version-annotation",
					},
				},
			},
		},
		&v1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					"admission.datadoghq.com/js-lib.version": "my-custom-val",
				},
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name: "applies-here",
					},
					{
						Name: "applies-here-too",
					},
				},
			},
		},
		"expected only js for both containers",
	)

	requireContainersConfig(t, map[string]LibraryConfig{
		"applies-here": {
			Languages: map[language]LanguageInfo{
				js: {
					Image:  "registry/dd-lib-js-init:my-custom-val",
					Source: "custom-container-lib-version-annotation",
				},
			},
		},
	}, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"admission.datadoghq.com/applies-here.js-lib.version": "my-custom-val",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "applies-here",
				},
				{
					Name: "not-here",
				},
			},
		},
	}, "only a single container should be set")

	requireContainersConfig(t, map[string]LibraryConfig{
		"applies-here": {
			Languages: map[language]LanguageInfo{
				js:   js.defaultLanguageInfo(registry),
				java: java.defaultLanguageInfo(registry),
			},
		},
		"and-here": {
			Languages: map[language]LanguageInfo{
				js:   js.defaultLanguageInfo(registry),
				java: java.defaultLanguageInfo(registry),
			},
		},
	}, &v1.Pod{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "applies-here",
				},
				{
					Name: "and-here",
				},
			},
		},
	}, "you get all languages with no overrides")

	podWithConflicts := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"admission.datadoghq.com/applies-here.js-lib.version": "my-custom-val",
				"admission.datadoghq.com/js-lib.version":              "my-custom-other-val",
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "applies-here",
				},
				{
					Name: "and-here",
				},
			},
		},
	}
	requireContainersConfig(t, map[string]LibraryConfig{
		"applies-here": {
			Languages: map[language]LanguageInfo{
				js: {
					Image:  "registry/dd-lib-js-init:my-custom-val",
					Source: "custom-container-lib-version-annotation",
				},
			},
		},
		"and-here": {
			Languages: map[language]LanguageInfo{
				js: {
					Image:  "registry/dd-lib-js-init:my-custom-other-val",
					Source: "custom-lib-version-annotation",
				},
			},
		},
	}, podWithConflicts, "containers have different language versions")

	// This is a test-case where we should probably error out but right now we don't!
	// We should _not_ be injecting more than one version per language as we cannot
	// support this right now.
	//
	// We can do an order of precedence for a specific container, that's fine
	// but not when different annotations really mean different things.
	requireLanguagesConfig(t, map[language]map[string]struct{}{
		js: map[string]struct{}{
			"registry/dd-lib-js-init:my-custom-other-val": struct{}{},
			"registry/dd-lib-js-init:my-custom-val": struct{}{},
		},
	}, podWithConflicts, "we know we have multiple images for a specific langauge")
}
