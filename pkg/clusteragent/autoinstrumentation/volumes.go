package autoinstrumentation

import (
	v1 "k8s.io/api/core/v1"
)

const (
	sharedLibVolumeName = "datadog-auto-instrumentation"
	sharedLibMountPath  = "/datadog-lib"
)

var (
	sharedLibVolume = newTmpVolume(sharedLibVolumeName)
	inspectVolume   = newTmpVolume("dd-entry-data")
)

type volume struct {
	v1.Volume
}

func newTmpVolume(name string) volume {
	return volume{
		Volume: v1.Volume{
			Name: name,
			VolumeSource: v1.VolumeSource{
				EmptyDir: &v1.EmptyDirVolumeSource{},
			},
		},
	}
}

func (v volume) mount(mountAt, subPath string) v1.VolumeMount {
	return v1.VolumeMount{
		Name:      v.Volume.Name,
		MountPath: mountAt,
		SubPath:   subPath,
	}
}
