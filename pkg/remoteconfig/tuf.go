package remoteconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/theupdateframework/go-tuf/client"
	"github.com/theupdateframework/go-tuf/data"
	"github.com/theupdateframework/go-tuf/util"
	"github.com/theupdateframework/go-tuf/verify"
)

type rootClientRemoteStore struct {
	roots [][]byte
}

func (s *rootClientRemoteStore) GetMeta(name string) (stream io.ReadCloser, size int64, err error) {
	metaPath, err := parseMetaPath(name)
	if err != nil {
		return nil, 0, err
	}
	if metaPath.role != roleRoot || !metaPath.versionSet {
		return nil, 0, client.ErrNotFound{File: name}
	}
	for _, root := range s.roots {
		parsedRoot, err := unsafeUnmarshalRoot(root)
		if err != nil {
			return nil, 0, err
		}
		if parsedRoot.Version == metaPath.version {
			return ioutil.NopCloser(bytes.NewReader(root)), int64(len(root)), nil
		}
	}
	return nil, 0, client.ErrNotFound{File: name}
}

func (s *rootClientRemoteStore) GetTarget(path string) (stream io.ReadCloser, size int64, err error) {
	return nil, 0, client.ErrNotFound{File: path}
}

type role string

const (
	roleRoot role = "root"
)

type metaPath struct {
	role       role
	version    int64
	versionSet bool
}

func parseMetaPath(rawMetaPath string) (metaPath, error) {
	splitRawMetaPath := strings.SplitN(rawMetaPath, ".", 3)
	if len(splitRawMetaPath) != 2 && len(splitRawMetaPath) != 3 {
		return metaPath{}, fmt.Errorf("invalid metadata path '%s'", rawMetaPath)
	}
	suffix := splitRawMetaPath[len(splitRawMetaPath)-1]
	if suffix != "json" {
		return metaPath{}, fmt.Errorf("invalid metadata path (suffix) '%s'", rawMetaPath)
	}
	rawRole := splitRawMetaPath[len(splitRawMetaPath)-2]
	if rawRole == "" {
		return metaPath{}, fmt.Errorf("invalid metadata path (role) '%s'", rawMetaPath)
	}
	if len(splitRawMetaPath) == 2 {
		return metaPath{
			role: role(rawRole),
		}, nil
	}
	rawVersion, err := strconv.ParseInt(splitRawMetaPath[0], 10, 64)
	if err != nil {
		return metaPath{}, fmt.Errorf("invalid metadata path (version) '%s': %w", rawMetaPath, err)
	}
	return metaPath{
		role:       role(rawRole),
		version:    rawVersion,
		versionSet: true,
	}, nil
}

func validateTargetFileHash(targetMeta data.TargetFileMeta, targetFile []byte) error {
	if len(targetMeta.HashAlgorithms()) == 0 {
		return fmt.Errorf("target file has no hash")
	}
	generatedMeta, err := util.GenerateFileMeta(bytes.NewBuffer(targetFile), targetMeta.HashAlgorithms()...)
	if err != nil {
		return err
	}
	err = util.FileMetaEqual(targetMeta.FileMeta, generatedMeta)
	if err != nil {
		return err
	}
	return nil
}

func unmarshalTargets(root *data.Root, rawTargets []byte) (*data.Targets, error) {
	db := verify.NewDB()
	for _, key := range root.Keys {
		for _, id := range key.IDs() {
			if err := db.AddKey(id, key); err != nil {
				return nil, err
			}
		}
	}
	targetsRole, hasRoleTargets := root.Roles["targets"]
	if !hasRoleTargets {
		return nil, fmt.Errorf("root is missing a targets role")
	}
	role := &data.Role{Threshold: targetsRole.Threshold, KeyIDs: targetsRole.KeyIDs}
	if err := db.AddRole("targets", role); err != nil {
		return nil, fmt.Errorf("could not add targets role to db: %v", err)
	}
	var targets data.Targets
	err := db.Unmarshal(rawTargets, &targets, "targets", 0)
	if err != nil {
		return nil, err
	}
	return &targets, nil
}

func unsafeUnmarshalRoot(raw []byte) (*data.Root, error) {
	var signedRoot data.Signed
	err := json.Unmarshal(raw, &signedRoot)
	if err != nil {
		return nil, err
	}
	var root data.Root
	err = json.Unmarshal(signedRoot.Signed, &root)
	if err != nil {
		return nil, err
	}
	return &root, err
}
