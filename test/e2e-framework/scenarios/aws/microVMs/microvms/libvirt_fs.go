// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package microvms

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/microvms/resources"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/microVMs/vmconfig"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	refreshFromEBS = "fio --filename=%s --rw=read --bs=64m --iodepth=32 --ioengine=libaio --direct=1 --name=volume-initialize"
	RootMountpoint = "/"
)

//go:embed files/download-vm-images.py
var downloadScriptContents string

type LibvirtFilesystem struct {
	ctx           *pulumi.Context
	pools         map[vmconfig.PoolType]LibvirtPool
	volumes       []LibvirtVolume
	baseVolumeMap map[string][]LibvirtVolume
	fsNamer       namer.Namer
	isLocal       bool
}

// libvirt complains when volume name contains '/'. We replace with '-'
func fsPathToLibvirtResource(path string) string {
	return strings.TrimPrefix(strings.ReplaceAll(path, "/", "-"), "-")
}

// the vmset name deduplicates volume resource name for the same VMs launched in different vmsets
// the architecture deduplicates volume resource name for the same VMs launched with different archs.
func getNamer(ctx *pulumi.Context, vmsetID vmconfig.VMSetID) func(string) namer.Namer {
	return func(volKey string) namer.Namer {
		return libvirtResourceNamer(ctx, fsPathToLibvirtResource(volKey), string(vmsetID))
	}
}

func buildVolumeResourceXMLFn(base map[string]pulumi.StringInput, recipe string) func(string, vmconfig.PoolType) pulumi.StringOutput {
	rc := resources.NewResourceCollection(recipe)
	return func(volumeKey string, poolType vmconfig.PoolType) pulumi.StringOutput {
		base[resources.VolumeKey] = pulumi.String(volumeKey)
		return rc.GetVolumeXML(&resources.RecipeLibvirtVolumeArgs{
			PoolType: poolType,
			XMLArgs:  base,
		})
	}
}

func isQCOW2(name string) bool {
	return strings.HasSuffix(name, "qcow2")
}

func format(name string) string {
	if isQCOW2(name) {
		return "qcow2"
	}

	return "raw"
}

// vms created with the distro recipe can have different backing filesystem images for different VMs.
// For example ubuntu and fedora VMs would have different backing images.
func NewLibvirtFSDistroRecipe(ctx *pulumi.Context, vmset *vmconfig.VMSet, pools map[vmconfig.PoolType]LibvirtPool) *LibvirtFilesystem {
	var volumes []LibvirtVolume

	baseVolumeMap := make(map[string][]LibvirtVolume)
	defaultPool := pools[resources.DefaultPool]
	for _, k := range vmset.Kernels {
		imageName := defaultPool.Name() + "-" + k.Tag
		imagePath := filepath.Join(GetWorkingDirectory(vmset.Arch), "rootfs", k.Dir)
		vol := NewLibvirtVolume(
			defaultPool,
			filesystemImage{
				imageName:   imageName,
				imagePath:   imagePath,
				imageSource: k.ImageSource,
			},
			buildVolumeResourceXMLFn(
				map[string]pulumi.StringInput{
					resources.ImageName: pulumi.String(imageName),
					resources.ImagePath: pulumi.String(imagePath),
					resources.Format:    pulumi.String(format(imagePath)),
				},
				vmset.Recipe,
			),
			getNamer(ctx, vmset.ID),
			RootMountpoint,
		)
		volumes = append(volumes, vol)
		baseVolumeMap[k.Tag] = append(baseVolumeMap[k.Tag], vol)
	}

	for _, d := range vmset.Disks {
		imgName := filepath.Base(d.Target)
		imageName := pools[d.Type].Name() + "-" + imgName
		vol := NewLibvirtVolume(
			pools[d.Type],
			filesystemImage{
				imageName:   imageName,
				imagePath:   d.Target,
				imageSource: d.BackingStore,
			},
			buildVolumeResourceXMLFn(
				map[string]pulumi.StringInput{
					resources.ImageName: pulumi.String(imageName),
					resources.ImagePath: pulumi.String(d.Target),
					resources.Format:    pulumi.String(format(imageName)),
				},
				vmset.Recipe,
			),
			getNamer(ctx, vmset.ID),
			d.Mountpoint,
		)

		// associate extra disks with all vms
		for _, k := range vmset.Kernels {
			baseVolumeMap[k.Tag] = append(baseVolumeMap[k.Tag], vol)
		}

		volumes = append(volumes, vol)
	}

	return &LibvirtFilesystem{
		ctx:           ctx,
		pools:         pools,
		volumes:       volumes,
		baseVolumeMap: baseVolumeMap,
		fsNamer:       libvirtResourceNamer(ctx, vmset.Tags...),
		isLocal:       vmset.Arch == LocalVMSet,
	}
}

// vms created with the custom recipe all share the same debian based backing filesystem image.
func NewLibvirtFSCustomRecipe(ctx *pulumi.Context, vmset *vmconfig.VMSet, pools map[vmconfig.PoolType]LibvirtPool) *LibvirtFilesystem {
	var volumes []LibvirtVolume

	baseVolumeMap := make(map[string][]LibvirtVolume)
	imageName := vmset.Img.ImageName
	path := filepath.Join(filepath.Join(GetWorkingDirectory(vmset.Arch), "rootfs"), imageName)
	vol := NewLibvirtVolume(
		pools[resources.DefaultPool],
		filesystemImage{
			imageName:   imageName,
			imagePath:   path,
			imageSource: vmset.Img.ImageSourceURI,
		},
		buildVolumeResourceXMLFn(
			map[string]pulumi.StringInput{
				resources.ImageName: pulumi.String(imageName),
				resources.ImagePath: pulumi.String(path),
				resources.Format:    pulumi.String(format(path)),
			},
			vmset.Recipe,
		),
		getNamer(ctx, vmset.ID),
		RootMountpoint,
	)
	volumes = append(volumes, vol)

	for _, k := range vmset.Kernels {
		baseVolumeMap[k.Tag] = append(baseVolumeMap[k.Tag], vol)
	}

	for _, d := range vmset.Disks {
		imgName := filepath.Base(d.Target)
		imageName := pools[d.Type].Name() + "-" + imgName
		vol := NewLibvirtVolume(
			pools[d.Type],
			filesystemImage{
				imageName:   imageName,
				imagePath:   d.Target,
				imageSource: d.BackingStore,
			},
			buildVolumeResourceXMLFn(
				map[string]pulumi.StringInput{
					resources.ImageName: pulumi.String(imageName),
					resources.ImagePath: pulumi.String(d.Target),
					resources.Format:    pulumi.String(format(imageName)),
				},
				vmset.Recipe,
			),
			getNamer(ctx, vmset.ID),
			d.Mountpoint,
		)

		// associate extra disks with all vms
		for _, k := range vmset.Kernels {
			baseVolumeMap[k.Tag] = append(baseVolumeMap[k.Tag], vol)
		}

		volumes = append(volumes, vol)
	}

	return &LibvirtFilesystem{
		ctx:           ctx,
		volumes:       volumes,
		baseVolumeMap: baseVolumeMap,
		pools:         pools,
		fsNamer:       libvirtResourceNamer(ctx, vmset.Tags...),
		isLocal:       vmset.Arch == LocalVMSet,
	}
}

func refreshFromBackingStore(volume LibvirtVolume, runner command.Runner, urlPath string, isLocal bool, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	var downloadCmd string
	var refreshCmd string

	fsImage := volume.UnderlyingImage()
	if isLocal {
		// For local environment we do not need to "download" the image from
		// a backing store.
		refreshCmd = "true"
	} else {
		refreshCmd = fmt.Sprintf(refreshFromEBS, urlPath)
	}

	// We do this because reading the EBS blocks is the only way to download the files
	// from the backing storage. Not doing this means, that the file is downloaded when
	// it is first accessed in other commands. This can cause other problems, on top of
	// being very slow.
	if urlPath != fsImage.imagePath {
		downloadCmd = fmt.Sprintf("%s && mv %s %s", refreshCmd, urlPath, fsImage.imagePath)
	} else {
		downloadCmd = refreshCmd
	}
	downloadRootfsArgs := command.Args{
		Create: pulumi.String(downloadCmd),
		Delete: pulumi.Sprintf("rm -f %s", fsImage.imagePath),
	}

	res, err := runner.Command(volume.FullResourceName("download-rootfs"), &downloadRootfsArgs, pulumi.DependsOn(depends))
	if err != nil {
		return nil, err
	}

	return []pulumi.Resource{res}, err
}

func downloadAndExtractRootfs(fs *LibvirtFilesystem, runner command.Runner, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource
	var downloadSpecs []filesystemImageDownload
	var retrieveImage []pulumi.Resource
	var imagesToExtract []*filesystemImage

	for _, volume := range fs.volumes {
		// only download backing stores for volumes inside default pool since these are
		// the images from which VMs boot
		//
		// ignore other volume types since they are created by this scenario and not downloaded.
		if volume.Pool().Type() != resources.DefaultPool {
			continue
		}

		fsImage := volume.UnderlyingImage()
		url, err := url.Parse(fsImage.imageSource)
		if err != nil {
			return nil, fmt.Errorf("error parsing url %s: %w", fsImage.imageSource, err)
		}

		if url.Scheme == "file" {
			retrieveImage, err = refreshFromBackingStore(volume, runner, url.Path, fs.isLocal, depends)
			if err != nil {
				return waitFor, err
			}
		} else {
			downloadSpecs = append(downloadSpecs, fsImage.toDownloadSpec())
		}

		if fsImage.isCompressed() {
			imagesToExtract = append(imagesToExtract, fsImage)
		}
	}

	if len(downloadSpecs) > 0 {
		var retrievePrepare []pulumi.Resource

		downloadSpecJSON, err := json.Marshal(downloadSpecs)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal to JSON download specs: %w", err)
		}

		downloadSpecsPath := fmt.Sprintf("/tmp/download-specs-%s.json", fs.fsNamer.ResourceName("download-specs"))
		writeConfigFile := command.Args{
			Create: pulumi.Sprintf("echo '%s' > %s", string(downloadSpecJSON), downloadSpecsPath),
			Update: pulumi.Sprintf("echo '%s' > %s", string(downloadSpecJSON), downloadSpecsPath),
			Delete: pulumi.Sprintf("rm -f %s", downloadSpecsPath),
		}
		writeConfigDone, err := runner.Command(fs.fsNamer.ResourceName("write-download-specs"), &writeConfigFile, pulumi.DependsOn(depends))
		if err != nil {
			return retrieveImage, err
		}
		retrievePrepare = append(retrievePrepare, writeConfigDone)

		downloadScriptFile := fmt.Sprintf("/tmp/download-vm-images-%s.py", fs.fsNamer.ResourceName("download-vm-images-script"))
		copyDownloadScriptFile := command.Args{
			Create: pulumi.Sprintf("echo '%s' > %s", downloadScriptContents, downloadScriptFile),
			Update: pulumi.Sprintf("echo '%s' > %s", downloadScriptContents, downloadScriptFile),
			Delete: pulumi.Sprintf("rm -f %s", downloadScriptFile),
		}
		copyDownloadScriptDone, err := runner.Command(fs.fsNamer.ResourceName("copy-download-script"), &copyDownloadScriptFile, pulumi.DependsOn(depends))
		if err != nil {
			return retrieveImage, err
		}
		retrievePrepare = append(retrievePrepare, copyDownloadScriptDone)

		downloadImagesArgs := command.Args{
			Create: pulumi.Sprintf("python3 %s %s", downloadScriptFile, downloadSpecsPath),
		}
		downloadImagesDone, err := runner.Command(fs.fsNamer.ResourceName("download-vm-images"), &downloadImagesArgs, pulumi.DependsOn(retrievePrepare))
		if err != nil {
			return retrieveImage, err
		}
		retrieveImage = append(retrieveImage, downloadImagesDone)
	}

	for _, fsImage := range imagesToExtract {
		extractImage, err := extractImage(fsImage, runner, fs.fsNamer.WithPrefix(fsImage.imageName), retrieveImage)
		if err != nil {
			return waitFor, err
		}
		waitFor = append(waitFor, extractImage...)
	}

	return waitFor, nil
}

func extractImage(fsImage *filesystemImage, runner command.Runner, namer namer.Namer, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	// Extract archive from the download path assuming it is xz compressed
	extractTopLevelArchive := command.Args{
		Create: pulumi.Sprintf("rm %s || true; xz -d %s", fsImage.imagePath, fsImage.downloadPath()),
	}
	res, err := runner.Command(namer.ResourceName("extract-base-volume-package"), &extractTopLevelArchive, pulumi.DependsOn(depends))
	if err != nil {
		return []pulumi.Resource{}, err
	}

	return []pulumi.Resource{res}, nil
}

func (fs *LibvirtFilesystem) SetupLibvirtFilesystem(providerFn LibvirtProviderFn, runner command.Runner, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	// Downloading the base images for the volumes is the slowest part of the entire setup.
	// We want this step to start as soon as our remote VMs are ready. Therefore, we do not
	// make it depend on any other step.
	//
	// [IMPORTANT] The download may start as the first step. So if the setup changes such that the download
	// becomes dependent on some prior step, this call should change !!
	downloadExtractRootfsDone, err := downloadAndExtractRootfs(fs, runner, nil)
	if err != nil {
		return nil, err
	}

	depends = append(depends, downloadExtractRootfsDone...)
	return setupLibvirtFilesystem(fs, runner, providerFn, depends)
}

func setupLibvirtFilesystem(fs *LibvirtFilesystem, runner command.Runner, providerFn LibvirtProviderFn, depends []pulumi.Resource) ([]pulumi.Resource, error) {
	var waitFor []pulumi.Resource
	for _, vol := range fs.volumes {
		setupLibvirtVMVolumeDone, err := vol.SetupLibvirtVMVolume(fs.ctx, runner, providerFn, fs.isLocal, depends)
		if err != nil {
			return nil, err
		}

		waitFor = append(waitFor, setupLibvirtVMVolumeDone)
	}

	return waitFor, nil
}
