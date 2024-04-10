// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package updater

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	oci "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/DataDog/datadog-agent/pkg/updater/repository"
	"github.com/DataDog/datadog-agent/pkg/updater/service"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	datadogPackageLayerMediaType       types.MediaType = "application/vnd.datadog.package.layer.v1.tar+zstd"
	datadogPackageConfigLayerMediaType types.MediaType = "application/vnd.datadog.package.config.layer.v1.tar+zstd"
	datadogPackageMaxSize                              = 3 << 30 // 3GiB
	defaultConfigsDir                                  = "/etc"
)

type installer struct {
	repositories *repository.Repositories
	configsDir   string
}

func newInstaller(repositories *repository.Repositories) *installer {
	return &installer{
		repositories: repositories,
		configsDir:   defaultConfigsDir,
	}
}

func (i *installer) installStable(pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.configsDir, pkg)
	err = extractPackageLayers(image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	err = i.repositories.Create(pkg, version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not create repository: %w", err)
	}
	if pkg == "datadog-agent" {
		return service.SetupAgentUnits()
	}
	return nil
}

func (i *installer) installExperiment(pkg string, version string, image oci.Image) error {
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		return fmt.Errorf("could not create temporary directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	configDir := filepath.Join(i.configsDir, pkg)
	err = extractPackageLayers(image, configDir, tmpDir)
	if err != nil {
		return fmt.Errorf("could not extract package layers: %w", err)
	}
	repository := i.repositories.Get(pkg)
	err = repository.SetExperiment(version, tmpDir)
	if err != nil {
		return fmt.Errorf("could not set experiment: %w", err)
	}
	return i.startExperiment(pkg)
}

func (i *installer) promoteExperiment(pkg string) error {
	repository := i.repositories.Get(pkg)
	err := repository.PromoteExperiment()
	if err != nil {
		return fmt.Errorf("could not promote experiment: %w", err)
	}
	return i.stopExperiment(pkg)
}

func (i *installer) uninstallExperiment(pkg string) error {
	repository := i.repositories.Get(pkg)
	err := repository.DeleteExperiment()
	if err != nil {
		return fmt.Errorf("could not delete experiment: %w", err)
	}
	return i.stopExperiment(pkg)
}

func (i *installer) startExperiment(pkg string) error {
	// TODO(arthur): currently we only support the datadog-agent package
	if pkg != "datadog-agent" {
		return nil
	}
	return service.StartAgentExperiment()
}

func (i *installer) stopExperiment(pkg string) error {
	// TODO(arthur): currently we only support the datadog-agent package
	if pkg != "datadog-agent" {
		return nil
	}
	return service.StopAgentExperiment()
}

func extractPackageLayers(image oci.Image, configDir string, packageDir string) error {
	layers, err := image.Layers()
	if err != nil {
		return fmt.Errorf("could not get image layers: %w", err)
	}
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			return fmt.Errorf("could not get layer media type: %w", err)
		}
		switch mediaType {
		case datadogPackageLayerMediaType:
			uncompressedLayer, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not uncompress layer: %w", err)
			}
			err = extractTarArchive(uncompressedLayer, packageDir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		case datadogPackageConfigLayerMediaType:
			uncompressedLayer, err := layer.Uncompressed()
			if err != nil {
				return fmt.Errorf("could not uncompress layer: %w", err)
			}
			err = extractTarArchive(uncompressedLayer, configDir, datadogPackageMaxSize)
			if err != nil {
				return fmt.Errorf("could not extract layer: %w", err)
			}
		default:
			log.Warnf("can't install unsupported layer media type: %s", mediaType)
		}
	}
	return nil
}

// extractTarArchive extracts a tar archive to the given destination path
//
// Note on security: This function does not currently attempt to fully mitigate zip-slip attacks.
// This is purposeful as the archive is extracted only after its SHA256 hash has been validated
// against its reference in the package catalog. This catalog is itself sent over Remote Config
// which guarantees its integrity.
func extractTarArchive(reader io.Reader, destinationPath string, maxSize int64) error {
	log.Debugf("Extracting archive to %s", destinationPath)
	tr := tar.NewReader(io.LimitReader(reader, maxSize))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("could not read tar header: %w", err)
		}
		if header.Name == "./" {
			continue
		}

		target := filepath.Join(destinationPath, header.Name)

		// Check for directory traversal. Note that this is more of a sanity check than a security measure.
		if !strings.HasPrefix(target, filepath.Clean(destinationPath)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry %s is trying to escape the destination directory", header.Name)
		}

		// Extract element depending on its type
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("could not create directory: %w", err)
			}
		case tar.TypeReg:
			err = extractTarFile(target, tr, os.FileMode(header.Mode))
			if err != nil {
				return err // already wrapped
			}
		case tar.TypeSymlink:
			err = os.Symlink(header.Linkname, target)
			if err != nil {
				return fmt.Errorf("could not create symlink: %w", err)
			}
		case tar.TypeLink:
			// we currently don't support hard links in the updater
		default:
			log.Warnf("Unsupported tar entry type %d for %s", header.Typeflag, header.Name)
		}
	}

	log.Debugf("Successfully extracted archive to %s", destinationPath)
	return nil
}

// extractTarFile extracts a file from a tar archive.
// It is separated from extractTarGz to ensure `defer f.Close()` is called right after the file is written.
func extractTarFile(targetPath string, reader io.Reader, mode fs.FileMode) error {
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return fmt.Errorf("could not create directory: %w", err)
	}
	f, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("could not create file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("could not write file: %w", err)
	}
	return nil
}
