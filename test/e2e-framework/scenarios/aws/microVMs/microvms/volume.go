// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
	"github.com/pulumi/pulumi-libvirt/sdk/go/libvirt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type LibvirtVolume interface {
	SetupLibvirtVMVolume(ctx *pulumi.Context, runner command.Runner, providerFn LibvirtProviderFn, isLocal bool, depends []pulumi.Resource) (pulumi.Resource, error)
	UnderlyingImage() *filesystemImage
	FullResourceName(...string) string
	Key() string
	Pool() LibvirtPool
	Mountpoint() string
}

type filesystemImage struct {
	imageName   string // name of the image
	imagePath   string // path on the image in the target filesystem after download and decompression if needed
	imageSource string // source URL of the image. Can end with .xz to indicate compression, image will be decompressed on download.
}

func (fsi *filesystemImage) isCompressed() bool {
	return strings.HasSuffix(fsi.imageSource, ".xz")
}

// downloadPath returns the path where the image will be downloaded to on the target filesystem.
// If the image is compressed, the path will have a .xz extension and it will be an intermediate file
// until it is decompressed.
func (fsi *filesystemImage) downloadPath() string {
	if fsi.isCompressed() {
		return fsi.imagePath + ".xz"
	}
	return fsi.imagePath
}

// checksumSource returns the source URL of the checksum file for the image.
func (fsi *filesystemImage) checksumSource() string {
	return fsi.imageSource + ".sum"
}

// checksumPath returns the path where the checksum file will be downloaded to on the target filesystem.
func (fsi *filesystemImage) checksumPath() string {
	return fsi.imagePath + ".sum"
}

type filesystemImageDownload struct {
	ImageName      string `json:"image_name"`
	ImagePath      string `json:"image_path"`
	ImageSource    string `json:"image_source"`
	ChecksumSource string `json:"checksum_source"`
	ChecksumPath   string `json:"checksum_path"`
}

func (fsi *filesystemImage) toDownloadSpec() filesystemImageDownload {
	return filesystemImageDownload{
		ImageName:      fsi.imageName,
		ImagePath:      fsi.downloadPath(),
		ImageSource:    fsi.imageSource,
		ChecksumSource: fsi.checksumSource(),
		ChecksumPath:   fsi.checksumPath(),
	}
}

type volume struct {
	filesystemImage
	pool        LibvirtPool
	volumeKey   string
	volumeXML   pulumi.StringOutput
	volumeNamer namer.Namer
	mountpoint  string
}

func generateVolumeKey(poolPath string, volName string) string {
	return fmt.Sprintf("%s/%s", poolPath, volName)
}

func NewLibvirtVolume(
	pool LibvirtPool,
	fsImage filesystemImage,
	xmlDataFn func(string, vmconfig.PoolType) pulumi.StringOutput,
	volNamerFn func(string) namer.Namer,
	mountpoint string,
) LibvirtVolume {
	volKey := generateVolumeKey(pool.Path(), fsImage.imageName)
	return &volume{
		filesystemImage: fsImage,
		volumeKey:       volKey,
		volumeXML:       xmlDataFn(volKey, pool.Type()),
		volumeNamer:     volNamerFn(fsImage.imageName),
		pool:            pool,
		mountpoint:      mountpoint,
	}
}

func remoteLibvirtVolume(v *volume, runner command.Runner, depends []pulumi.Resource) (pulumi.Resource, error) {
	var baseVolumeReady pulumi.Resource

	volumeXMLPath := fmt.Sprintf("/tmp/volume-%s.xml", v.filesystemImage.imageName)
	volXMLWrittenArgs := command.Args{
		Create: pulumi.Sprintf("echo \"%s\" > %s", v.volumeXML, volumeXMLPath),
		Delete: pulumi.Sprintf("rm -f %s", volumeXMLPath),
	}
	// XML write does not need to depend on anything other than the instance being ready.
	// Instance state is handled by the runner automatically.
	volXMLWritten, err := runner.Command(
		v.volumeNamer.ResourceName("write-vol-xml"),
		&volXMLWrittenArgs,
	)
	if err != nil {
		return baseVolumeReady, err
	}

	depends = append(depends, volXMLWritten)

	baseVolumeReadyArgs := command.Args{
		Create: pulumi.Sprintf("virsh vol-create %s %s", v.pool.Name(), volumeXMLPath),
		Delete: pulumi.Sprintf("virsh vol-delete %s --pool %s", v.volumeKey, v.pool.Name()),
		Sudo:   true,
	}

	baseVolumeReady, err = runner.Command(v.volumeNamer.ResourceName("build-libvirt-basevolume"), &baseVolumeReadyArgs, pulumi.DependsOn(depends))
	if err != nil {
		return baseVolumeReady, err
	}

	return baseVolumeReady, err
}

func localLibvirtVolume(v *volume, ctx *pulumi.Context, providerFn LibvirtProviderFn, depends []pulumi.Resource) (pulumi.Resource, error) {
	var stgvolReady pulumi.Resource

	provider, err := providerFn()
	if err != nil {
		return stgvolReady, err
	}

	stgvolReady, err = libvirt.NewVolume(ctx, v.volumeNamer.ResourceName("build-libvirt-basevolume"), &libvirt.VolumeArgs{
		Name:   pulumi.String(v.filesystemImage.imageName),
		Pool:   pulumi.String(v.pool.Name()),
		Source: pulumi.String(v.filesystemImage.imagePath),
		Xml: libvirt.VolumeXmlArgs{
			Xslt: v.volumeXML,
		},
	}, pulumi.Provider(provider), pulumi.DependsOn(depends))
	if err != nil {
		return stgvolReady, err
	}

	return stgvolReady, nil
}

func (v *volume) SetupLibvirtVMVolume(ctx *pulumi.Context, runner command.Runner, providerFn LibvirtProviderFn, isLocal bool, depends []pulumi.Resource) (pulumi.Resource, error) {
	if isLocal {
		return localLibvirtVolume(v, ctx, providerFn, depends)
	}

	return remoteLibvirtVolume(v, runner, depends)
}

func (v *volume) UnderlyingImage() *filesystemImage {
	return &v.filesystemImage
}

func (v *volume) FullResourceName(name ...string) string {
	return v.volumeNamer.ResourceName(name...)
}

func (v *volume) Key() string {
	return v.volumeKey
}

func (v *volume) Pool() LibvirtPool {
	return v.pool
}

func (v *volume) Mountpoint() string {
	return v.mountpoint
}
