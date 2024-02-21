// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "crypto/sha256"
	_ "crypto/sha512"

	digest "github.com/opencontainers/go-digest"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	bolt "go.etcd.io/bbolt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type containerMountpoint struct {
	ImageName     string
	ImageDigest   string
	ContainerName string
	Path          string
}

func mountContainers(ctx context.Context, scan *scanTask, root string) (mountPoints []containerMountpoint, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("mountContainers panic recovered: %s", r)
		}
	}()

	ctrdRoot := filepath.Join(root, "/var/lib/containerd")
	ctrdRootInfo, err := os.Stat(ctrdRoot)
	if err == nil && ctrdRootInfo.IsDir() {
		log.Debugf("%s: starting scanning for containerd containers", scan)
		containers, err := ctrdReadMetadata(ctrdRoot)
		if err != nil {
			return nil, err
		}
		log.Debugf("%s: found %d containers on %q", scan, len(containers), root)
		for _, ctr := range containers {
			if ctr.Snapshot.Backend.Kind != kindActive {
				continue
			}

			log.Debugf("%s: container %s", scan, ctr)
			if ctr.Snapshot == nil {
				log.Warnf("%s: container %s is active but without an associated snapshot", scan, ctr)
				continue
			}

			ctrMountName := fmt.Sprintf("%s%s-%s-%d", ctrdMountPrefix, ctr.NS, ctr.Name, ctr.Snapshot.Backend.ID)
			ctrLayers := ctrdLayersPaths(ctrdRoot, ctr.Snapshot)
			ctrMountPoint, err := mountContainer(ctx, scan, ctrMountName, ctrLayers)
			if err != nil {
				log.Errorf("%s: could not mount container %s: %v", scan, ctr, err)
				continue
			}
			mountPoints = append(mountPoints, containerMountpoint{
				ImageName:     ctr.ImageName,
				ImageDigest:   ctr.Image.Digest.String(),
				ContainerName: ctr.Name,
				Path:          ctrMountPoint,
			})
		}
	}

	dockerRoot := filepath.Join(root, "/var/lib/docker")
	dockerRootInfo, err := os.Stat(dockerRoot)
	if err == nil && dockerRootInfo.IsDir() {
		log.Debugf("%s: starting scanning for docker containers", scan)
		containers, err := dockerReadMetadata(dockerRoot)
		if err != nil {
			return nil, err
		}
		log.Debugf("%s: found %d containers on %q", scan, len(containers), root)
		for _, ctr := range containers {
			if !ctr.State.Running {
				continue
			}

			log.Debugf("%s: container %s", scan, ctr)

			ctrMountName := dockerMountPrefix + ctr.ID
			ctrLayers, err := dockerLayersPaths(dockerRoot, ctr)
			if err != nil {
				log.Errorf("could get container layers %s: %v", ctr, err)
				continue
			}
			ctrMountPoint, err := mountContainer(ctx, scan, ctrMountName, ctrLayers)
			if err != nil {
				log.Errorf("could not mount container %s: %v", ctr, err)
				continue
			}
			mountPoints = append(mountPoints, containerMountpoint{
				ImageName:     ctr.Config.Image,
				ImageDigest:   ctr.Image.String(),
				ContainerName: ctr.Name,
				Path:          ctrMountPoint,
			})
		}
	}

	return mountPoints, nil
}

func containerTags(ctr containerMountpoint) (string, []string) {
	imageNameSplit := strings.SplitN(ctr.ImageName, ":", 2)
	if len(imageNameSplit) == 1 {
		imageNameSplit = append(imageNameSplit, "")
	}
	imageRepo := imageNameSplit[0]
	imageRepoSplit := strings.Split(imageRepo, "/")
	entityID := imageRepo + "@" + ctr.ImageDigest
	entityTags := []string{
		"image_id:" + entityID,                                      // public.ecr.aws/datadog/agent@sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409
		"image_name:" + imageRepo,                                   // public.ecr.aws/datadog/agent
		"image_registry:" + imageRepoSplit[0],                       // public.ecr.aws
		"image_repository:" + strings.Join(imageRepoSplit[1:], "/"), // datadog/agent
		"short_image:" + imageRepoSplit[len(imageRepoSplit)-1],      // agent
		"repo_digest:" + ctr.ImageDigest,                            // sha256:052f1fdf4f9a7117d36a1838ab60782829947683007c34b69d4991576375c409
		"image_tag:" + imageNameSplit[1],                            // 7-rc
		"container_name:" + ctr.ContainerName,
	}
	return entityID, entityTags
}

func mountContainer(ctx context.Context, scan *scanTask, name string, ctrLayers []string) (string, error) {
	if len(ctrLayers) == 0 {
		return "", fmt.Errorf("container without any layer")
	}
	if len(ctrLayers) == 1 {
		// only one layer, no need to mount anything.
		return ctrLayers[0], nil
	}
	ctrMountPoint := scan.Path(name)
	if err := os.MkdirAll(ctrMountPoint, 0700); err != nil {
		return "", fmt.Errorf("could not create container mountPoint directory %q: %w", ctrMountPoint, err)
	}
	ctrMountOpts := []string{
		"-o", "ro,noauto,nodev,noexec,nosuid,index=off," + fmt.Sprintf("lowerdir=%s", strings.Join(ctrLayers, ":")),
		"-t", "overlay",
		"--source", "overlay",
		"--target", ctrMountPoint,
	}
	log.Debugf("execing mount %s", ctrMountOpts)
	mountOutput, err := exec.CommandContext(ctx, "mount", ctrMountOpts...).CombinedOutput()
	if err != nil {
		err = fmt.Errorf("could not mount into target=%q options=%q output=%q: %w", ctrMountPoint, ctrMountOpts, string(mountOutput), err)
		return "", err
	}
	return ctrMountPoint, nil
}

const ctrdSupportedVersion = 3

var (
	errCtrdInvalidState = fmt.Errorf("invalid state of the containerd databaase")
)

var (
	// reference: https://github.com/containerd/containerd/blob/f8b07365d260a69f22371964bb23cbcc73e23790/metadata/buckets.go
	bucketKeyVersion          = []byte("v1")
	bucketKeyDBVersion        = []byte("version")    // stores the version of the schema
	bucketKeyObjectLabels     = []byte("labels")     // stores the labels for a namespace.
	bucketKeyObjectImages     = []byte("images")     // stores image objects
	bucketKeyObjectContainers = []byte("containers") // stores container objects
	bucketKeyObjectSnapshots  = []byte("snapshots")  // stores snapshot references
	bucketKeyObjectContent    = []byte("content")    // stores content references
	bucketKeyObjectBlob       = []byte("blob")       // stores content links

	bucketKeyDigest      = []byte("digest")
	bucketKeyMediaType   = []byte("mediatype")
	bucketKeySize        = []byte("size")
	bucketKeyImage       = []byte("image")
	bucketKeyRuntime     = []byte("runtime")
	bucketKeyName        = []byte("name")
	bucketKeyParent      = []byte("parent")
	bucketKeyChildren    = []byte("children")
	bucketKeyOptions     = []byte("options")
	bucketKeySpec        = []byte("spec")
	bucketKeySnapshotKey = []byte("snapshotKey")
	bucketKeySnapshotter = []byte("snapshotter")
	bucketKeyTarget      = []byte("target")
	bucketKeyCreatedAt   = []byte("createdat")
	bucketKeyUpdatedAt   = []byte("updatedat")
)

type snapshotKind string

const (
	kindUnknown   snapshotKind = "Unknown"
	kindView      snapshotKind = "View"
	kindActive    snapshotKind = "Active"
	kindCommitted snapshotKind = "Committed"
)

type ctrdBlob struct {
	ID        digest.Digest
	Size      int64
	Labels    map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ctrdImage struct {
	NS           string
	Name         string
	Digest       digest.Digest
	MediaType    string
	Size         int64
	Blob         *ctrdBlob
	Labels       map[string]string
	ManifestList struct {
		Manifest []struct {
			Digest    digest.Digest `json:"digest"`
			MediaType string        `json:"mediaType"`
			Size      int64         `json:"size"`
			Platform  struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ctrdContainer struct {
	NS          string
	Name        string
	Snapshotter string
	SnapshotKey string
	Snapshot    *ctrdSnapshot
	Runtime     struct {
		Name           string
		OptionsTypeURL string
		Options        []byte
	}
	Labels    map[string]string
	ImageName string
	Image     *ctrdImage
	Spec      interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c ctrdContainer) String() string {
	return fmt.Sprintf("%s/%s", c.NS, c.Name)
}

type ctrdSnapshot struct {
	Name     string
	Parent   digest.Digest
	Children []digest.Digest
	Labels   map[string]string
	Backend  struct {
		ID        uint64
		Kind      snapshotKind
		Parents   []uint64
		Inodes    int64
		Size      int64
		CreatedAt time.Time
		UpdatedAt time.Time
	}
	CreatedAt time.Time
	UpdatedAt time.Time
}

func ctrdReadMetadata(ctrdRoot string) ([]ctrdContainer, error) {
	metadbPath := filepath.Join(ctrdRoot, "io.containerd.metadata.v1.bolt", "meta.db")
	metadbInfo, err := os.Stat(metadbPath)
	if err != nil || metadbInfo.Size() == 0 {
		return nil, nil
	}
	db, err := bolt.Open(metadbPath, 0600, &bolt.Options{
		ReadOnly: true,
	})
	if err != nil {
		return nil, err
	}
	defer db.Close()

	snapshotterDBPath := filepath.Join(ctrdRoot, "io.containerd.snapshotter.v1.overlayfs", "metadata.db")
	snapshotterDBInfo, err := os.Stat(snapshotterDBPath)
	if err != nil || snapshotterDBInfo.Size() == 0 {
		return nil, nil
	}
	snapshotterDB, err := bolt.Open(snapshotterDBPath, 0600, &bolt.Options{
		ReadOnly: true,
	})
	if err != nil {
		return nil, err
	}
	defer snapshotterDB.Close()

	var namespaces [][]byte
	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketKeyVersion)
		if bkt == nil {
			return errCtrdInvalidState
		}
		v, _ := binary.Varint(bkt.Get(bucketKeyDBVersion))
		if v != ctrdSupportedVersion {
			return errCtrdInvalidState
		}
		return bkt.ForEachBucket(func(ns []byte) error {
			namespaces = append(namespaces, ns)
			return nil
		})
	}); err != nil {
		return nil, err
	}

	var containers []ctrdContainer

	if err := db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket(bucketKeyVersion)
		if bkt == nil {
			return errCtrdInvalidState
		}
		for _, ns := range namespaces {
			images := make(map[string]*ctrdImage)
			blobs := make(map[digest.Digest]*ctrdBlob)
			bktNS := bkt.Bucket(ns)
			if bktNS == nil {
				return errCtrdInvalidState
			}

			bktContent := bktNS.Bucket(bucketKeyObjectContent)
			if bktContent == nil {
				return errCtrdInvalidState
			}
			bktBlobs := bktContent.Bucket(bucketKeyObjectBlob)
			if bktBlobs == nil {
				return errCtrdInvalidState
			}
			if err := bktBlobs.ForEachBucket(func(blobID []byte) error {
				bktBlob := bktBlobs.Bucket(blobID)
				if bktBlob == nil {
					return errCtrdInvalidState
				}

				var blob ctrdBlob
				blob.ID, err = digest.Parse(string(blobID))
				if err != nil {
					return err
				}
				blob.Size, _ = binary.Varint(bktBlob.Get(bucketKeySize))
				blob.Labels = make(map[string]string)
				if err := blob.CreatedAt.UnmarshalBinary(bktBlob.Get(bucketKeyCreatedAt)); err != nil {
					return err
				}
				if err := blob.UpdatedAt.UnmarshalBinary(bktBlob.Get(bucketKeyUpdatedAt)); err != nil {
					return err
				}
				if bktBlobLabels := bktBlob.Bucket(bucketKeyObjectLabels); bktBlobLabels != nil {
					if err := bktBlobLabels.ForEach(func(k, v []byte) error {
						blob.Labels[string(k)] = string(v)
						return nil
					}); err != nil {
						return err
					}
				}

				blobs[blob.ID] = &blob
				return nil
			}); err != nil {
				return err
			}

			bktImgs := bktNS.Bucket(bucketKeyObjectImages)
			if bktImgs == nil {
				return errCtrdInvalidState
			}

			if err := bktImgs.ForEachBucket(func(imageName []byte) error {
				bktImg := bktImgs.Bucket(imageName)
				if bktImg == nil {
					return errCtrdInvalidState
				}
				bktImageTarget := bktImg.Bucket(bucketKeyTarget)
				if bktImageTarget == nil {
					return errCtrdInvalidState
				}

				var image ctrdImage
				if err := image.CreatedAt.UnmarshalBinary(bktImg.Get(bucketKeyCreatedAt)); err != nil {
					return err
				}
				if err := image.UpdatedAt.UnmarshalBinary(bktImg.Get(bucketKeyUpdatedAt)); err != nil {
					return err
				}
				image.NS = string(ns)
				image.Name = string(imageName)
				image.Digest, err = digest.Parse(string(bktImageTarget.Get(bucketKeyDigest)))
				if err != nil {
					return err
				}
				image.MediaType = string(bktImageTarget.Get(bucketKeyMediaType))
				image.Size, _ = binary.Varint(bktImageTarget.Get(bucketKeySize))
				image.Blob = blobs[image.Digest]
				if image.Blob != nil {
					blobPath, err := ctrdBlobPath(ctrdRoot, image.Blob.ID)
					if err != nil {
						return err
					}
					blobContent, err := os.ReadFile(blobPath)
					if err != nil {
						return err
					}
					switch image.MediaType {
					case "application/vnd.docker.distribution.manifest.list.v2+json":
						if err := json.Unmarshal(blobContent, &image.ManifestList); err != nil {
							return err
						}
					}
				}
				image.Labels = make(map[string]string)
				if bktImgLabels := bktImg.Bucket(bucketKeyObjectLabels); bktImgLabels != nil {
					if err := bktImgLabels.ForEach(func(k, v []byte) error {
						image.Labels[string(k)] = string(v)
						return nil
					}); err != nil {
						return err
					}
				}

				images[image.Name] = &image
				return nil
			}); err != nil {
				return err
			}

			bktCtrs := bktNS.Bucket(bucketKeyObjectContainers)
			if bktCtrs == nil {
				return errCtrdInvalidState
			}
			if err := bktCtrs.ForEachBucket(func(containerName []byte) error {
				var container ctrdContainer

				bktCtr := bktCtrs.Bucket(containerName)
				if bktCtr == nil {
					return errCtrdInvalidState
				}

				var specPB anypb.Any
				if err := proto.Unmarshal(bktCtr.Get(bucketKeySpec), &specPB); err != nil {
					return err
				}
				if err := json.Unmarshal(specPB.GetValue(), &container.Spec); err != nil {
					return err
				}
				container.NS = string(ns)
				container.Name = string(containerName)
				container.ImageName = string(bktCtr.Get(bucketKeyImage))
				container.Image = images[container.ImageName]
				container.Snapshotter = string(bktCtr.Get(bucketKeySnapshotter))
				container.SnapshotKey = string(bktCtr.Get(bucketKeySnapshotKey))
				container.Labels = make(map[string]string)
				if err := container.CreatedAt.UnmarshalBinary(bktCtr.Get(bucketKeyCreatedAt)); err != nil {
					return err
				}
				if err := container.UpdatedAt.UnmarshalBinary(bktCtr.Get(bucketKeyUpdatedAt)); err != nil {
					return err
				}

				if bktRuntime := bktCtr.Bucket(bucketKeyRuntime); bktRuntime != nil {
					container.Runtime.Name = string(bktRuntime.Get(bucketKeyName))
					var options anypb.Any
					if err := proto.Unmarshal(bktRuntime.Get(bucketKeyOptions), &options); err == nil {
						container.Runtime.OptionsTypeURL = options.TypeUrl
						container.Runtime.Options = options.Value
					}
				}

				if bktCtrLabels := bktCtr.Bucket(bucketKeyObjectLabels); bktCtrLabels != nil {
					if err := bktCtrLabels.ForEach(func(k, v []byte) error {
						container.Labels[string(k)] = string(v)
						return nil
					}); err != nil {
						return err
					}
				}

				bktSnaps := bktNS.Bucket(bucketKeyObjectSnapshots)
				if bktSnaps == nil {
					return errCtrdInvalidState
				}
				switch container.Snapshotter {
				case "overlayfs":
					bktSnapshotter := bktSnaps.Bucket([]byte(container.Snapshotter))
					if bktSnapshotter == nil {
						return errCtrdInvalidState
					}
					bktSnap := bktSnapshotter.Bucket([]byte(container.SnapshotKey))
					if bktSnap == nil {
						return errCtrdInvalidState
					}
					var snapshot ctrdSnapshot
					snapshot.Name = string(bktSnap.Get(bucketKeyName))
					snapshot.Parent, err = digest.Parse(string(bktSnap.Get(bucketKeyParent)))
					if err != nil {
						return err
					}
					if err := snapshot.Parent.Validate(); err != nil {
						return err
					}
					snapshot.Labels = make(map[string]string)
					if err := snapshot.CreatedAt.UnmarshalBinary(bktSnap.Get(bucketKeyCreatedAt)); err != nil {
						return err
					}
					if err := snapshot.UpdatedAt.UnmarshalBinary(bktSnap.Get(bucketKeyUpdatedAt)); err != nil {
						return err
					}
					if bktChildren := bktSnap.Bucket(bucketKeyChildren); bktChildren != nil {
						if err := bktChildren.ForEach(func(k, _ []byte) error {
							child, err := digest.Parse(string(k))
							if err != nil {
								return err
							}
							snapshot.Children = append(snapshot.Children, child)
							return nil
						}); err != nil {
							return err
						}
					}
					if bktSnapLabels := bktSnap.Bucket(bucketKeyObjectLabels); bktSnapLabels != nil {
						if err := bktSnapLabels.ForEach(func(k, v []byte) error {
							snapshot.Labels[string(k)] = string(v)
							return nil
						}); err != nil {
							return err
						}
					}
					if err := ctrdFillSapshotBackend(snapshotterDB, &snapshot); err != nil {
						return err
					}
					container.Snapshot = &snapshot
				default:
					return fmt.Errorf("unsupported snapshotter %q", container.Snapshotter)
				}

				containers = append(containers, container)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return containers, nil
}

func ctrdBlobPath(ctrdRoot string, blobID digest.Digest) (string, error) {
	if err := blobID.Validate(); err != nil {
		return "", fmt.Errorf("invalid blob digest: %w", err)
	}
	blobPath := filepath.Join(ctrdRoot, "io.containerd.content.v1.content", "blobs", blobID.Algorithm().String(), blobID.Encoded())
	return blobPath, nil
}

func ctrdFillSapshotBackend(db *bolt.DB, snapshot *ctrdSnapshot) error {
	var (
		bucketKeyStorageVersion = []byte("v1")
		bucketKeySnapshot       = []byte("snapshots")

		bucketKeyID     = []byte("id")
		bucketKeyParent = []byte("parent")
		bucketKeyKind   = []byte("kind")
		bucketKeyInodes = []byte("inodes")
		bucketKeySize   = []byte("size")
	)

	return db.View(func(tx *bolt.Tx) error {
		bucketSchemaVersion := tx.Bucket(bucketKeyStorageVersion)
		if bucketSchemaVersion == nil {
			return errCtrdInvalidState
		}
		bktSnaps := bucketSchemaVersion.Bucket(bucketKeySnapshot)
		if bktSnaps == nil {
			return errCtrdInvalidState
		}
		bktSnap := bktSnaps.Bucket([]byte(snapshot.Name))
		if bktSnap == nil {
			return fmt.Errorf("could not find snapshot with key %q", snapshot.Name)
		}
		snapshot.Backend.ID, _ = binary.Uvarint(bktSnap.Get(bucketKeyID))
		bktSnapshotParent := bktSnap
		for {
			parentKey := bktSnapshotParent.Get(bucketKeyParent)
			if len(parentKey) == 0 {
				break
			}
			bktSnapshotParent = bktSnaps.Bucket([]byte(parentKey))
			if bktSnapshotParent == nil {
				break
			}
			parentID, _ := binary.Uvarint(bktSnapshotParent.Get(bucketKeyID))
			snapshot.Backend.Parents = append(snapshot.Backend.Parents, parentID)
		}
		if kind := bktSnap.Get(bucketKeyKind); len(kind) > 0 {
			switch kind[0] {
			case 1:
				snapshot.Backend.Kind = kindView
			case 2:
				snapshot.Backend.Kind = kindActive
			case 3:
				snapshot.Backend.Kind = kindCommitted
			default:
				snapshot.Backend.Kind = kindUnknown
			}
		}
		snapshot.Backend.Inodes, _ = binary.Varint(bktSnap.Get(bucketKeyInodes))
		snapshot.Backend.Size, _ = binary.Varint(bktSnap.Get(bucketKeySize))
		if err := snapshot.CreatedAt.UnmarshalBinary(bktSnap.Get(bucketKeyCreatedAt)); err != nil {
			return err
		}
		if err := snapshot.UpdatedAt.UnmarshalBinary(bktSnap.Get(bucketKeyUpdatedAt)); err != nil {
			return err
		}
		return nil
	})
}

func ctrdLayersPaths(ctrdRoot string, s *ctrdSnapshot) []string {
	mountLayers := make([]string, 0, len(s.Backend.Parents)+1)
	if s.Backend.Kind == kindActive {
		mountLayers = append(mountLayers, ctrdLayerPath(ctrdRoot, s.Backend.ID))
	}
	for _, parentID := range s.Backend.Parents {
		mountLayers = append(mountLayers, ctrdLayerPath(ctrdRoot, parentID))
	}
	return mountLayers
}

func ctrdLayerPath(ctrdRoot string, id uint64) string {
	return filepath.Join(ctrdRoot, "io.containerd.snapshotter.v1.overlayfs", "snapshots", strconv.FormatInt(int64(id), 10), "fs")
}

type dockerImage struct {
	Architecture string `json:"architecture"`
	RootFS       struct {
		Type    string          `json:"type"`
		DiffIDs []digest.Digest `json:"diff_ids"`
	} `json:"rootfs"`
	Variant string `json:"variant"`
}

type dockerContainer struct {
	ID      string    `json:"ID"`
	Created time.Time `json:"Created"`
	State   struct {
		Running    bool      `json:"Running"`
		StartedAt  time.Time `json:"StartedAt"`
		FinishedAt time.Time `json:"FinishedAt"`
	} `json:"State"`
	Config struct {
		Hostname string              `json:"Hostname"`
		Image    string              `json:"Image"`
		Volumes  map[string]struct{} `json:"Volumes"`
		Labels   map[string]string   `json:"Labels"`
	} `json:"Config"`

	Image         digest.Digest `json:"Image"`
	ImageManifest *dockerImage  `json:"-"`
	Name          string        `json:"Name"`
	Driver        string        `json:"Driver"`
	OS            string        `json:"OS"`
}

func (c dockerContainer) String() string {
	return fmt.Sprintf("%s/%s", c.ID, c.Name)
}

func dockerReadMetadata(dockerRoot string) ([]dockerContainer, error) {
	entries, err := os.ReadDir(filepath.Join(dockerRoot, "containers"))
	if err != nil {
		return nil, err
	}

	ctrSums := make([]string, 0, len(entries))
	for _, entry := range entries {
		ctrSums = append(ctrSums, entry.Name())
	}

	const maxFileSize = 2 * 1024 * 1024
	var containers []dockerContainer
	for _, ctrSum := range ctrSums {
		var ctr dockerContainer
		cfgPath := filepath.Join(dockerRoot, "containers", cleanPath(ctrSum), "config.v2.json")
		cfgData, err := readFileLimit(cfgPath, maxFileSize)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(cfgData, &ctr); err != nil {
			return nil, err
		}
		if err := ctr.Image.Validate(); err != nil {
			return nil, err
		}
		var img dockerImage
		imagePath := filepath.Join(dockerRoot, "image/overlay2/imagedb/content", ctr.Image.Algorithm().String(), ctr.Image.Encoded())
		imageData, err := readFileLimit(imagePath, maxFileSize)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(imageData, &img); err != nil {
			return nil, err
		}
		ctr.ImageManifest = &img
		containers = append(containers, ctr)
	}

	return containers, nil
}

func dockerLayersPaths(dockerRoot string, ctr dockerContainer) ([]string, error) {
	if ctr.Driver != "overlay2" {
		return nil, fmt.Errorf("unsupported docker container driver %q", ctr.Driver)
	}

	mountsPath := filepath.Join(dockerRoot, "image/overlay2/layerdb/mounts", cleanPath(ctr.ID))
	mountIDPath := filepath.Join(mountsPath, "mount-id")
	mountIDData, err := readFileLimit(mountIDPath, 256)
	if err != nil {
		return nil, err
	}

	initIDPath := filepath.Join(mountsPath, cleanPath(ctr.ID), "init-id")
	initIDData, err := readFileLimit(initIDPath, 256)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	var layers []string
	layers = append(layers, dockerLayerPath(dockerRoot, mountIDData))
	if len(initIDData) > 0 {
		layers = append(layers, dockerLayerPath(dockerRoot, initIDData))
	}
	for _, d := range ctr.ImageManifest.RootFS.DiffIDs {
		if err := d.Validate(); err != nil {
			return nil, err
		}
		cacheIDPath := filepath.Join(dockerRoot, "image/overlay2/layerdb", d.Algorithm().String(), d.Encoded(), "cache-id")
		cacheIDData, err := readFileLimit(cacheIDPath, 256)
		if err != nil {
			return nil, err
		}
		layers = append(layers, dockerLayerPath(dockerRoot, cacheIDData))
	}
	return layers, nil
}

func dockerLayerPath(dockerRoot string, id []byte) string {
	return filepath.Join(dockerRoot, "overlay2", cleanPath(string(id)), "diff")
}

func cleanPath(name string) string {
	return filepath.Join("/", name)[1:]
}

func readFileLimit(name string, n int64) ([]byte, error) {
	cfgInfo, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if cfgInfo.Size() > n {
		return nil, fmt.Errorf("read: file is too big %d (limit is %d)", cfgInfo.Size(), n)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := io.LimitReader(f, n)
	return io.ReadAll(r)
}
