// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"archive/tar"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/cilium/ebpf/btf"

	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/archive"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const btfFlushDelay = 1 * time.Minute
const btfArchiveName = "minimized-btfs.tar.xz"

type btfPlatform string

const (
	platformAmazon       btfPlatform = "amzn"
	platformCentOS       btfPlatform = "centos"
	platformDebian       btfPlatform = "debian"
	platformFedora       btfPlatform = "fedora"
	platformOpenSUSELeap btfPlatform = "opensuse-leap"
	platformOracle       btfPlatform = "ol"
	platformRedhat       btfPlatform = "rhel"
	platformSUSE         btfPlatform = "sles"
	platformUbuntu       btfPlatform = "ubuntu"
)

func (p btfPlatform) String() string {
	return string(p)
}

func btfPlatformFromString(platform string) (btfPlatform, error) {
	switch platform {
	case "amzn", "amazon":
		return platformAmazon, nil
	case "suse", "sles":
		return platformSUSE, nil
	case "opensuse", "opensuse-leap":
		return platformOpenSUSELeap, nil
	case "redhat", "rhel":
		return platformRedhat, nil
	case "oracle", "ol":
		return platformOracle, nil
	case "ubuntu":
		return platformUbuntu, nil
	case "centos":
		return platformCentOS, nil
	case "debian":
		return platformDebian, nil
	case "fedora":
		return platformFedora, nil
	default:
		return "", fmt.Errorf("%s unsupported", platform)
	}
}

// FlushBTF deletes any cache of loaded BTF data, regardless of how it was sourced.
func FlushBTF() {
	core.RLock()
	loader := core.loader
	core.RUnlock()

	if loader != nil {
		loader.btfLoader.Flush()
	} else {
		loadKernelSpec.Flush()
	}
}

type kernelModuleBTFLoadFunc func(string) (*btf.Spec, error)

type returnBTF struct {
	vmlinux        *btf.Spec
	moduleLoadFunc kernelModuleBTFLoadFunc
}

type BTFResultMetadata struct {
	// numLoadAttempts is how many times the loader has been invoked (doesn't include cached requests)
	numLoadAttempts int
	// loaderUsed the name of the loader that was used to get the BTF data
	loaderUsed string
	// filepathUsed is the filepath it last tried to load BTF files from (only for loadUser and loadEmbedded)
	filepathUsed string
	// tarballUsed is the filepath for the tarball it tried to extract BTF from (only for loadEmbedded)
	tarballUsed string
}

func (d BTFResultMetadata) String() string {
	res := fmt.Sprintf("numLoadAttempts: %d\nloaderUsed: %s", d.numLoadAttempts, d.loaderUsed)
	if d.filepathUsed != "" {
		res += fmt.Sprintf("\nfilepathUsed: %s", d.filepathUsed)
	}
	if d.tarballUsed != "" {
		res += fmt.Sprintf("\ntarballUsed: %s", d.tarballUsed)
	}
	return res
}

type orderedBTFLoader struct {
	userBTFPath string
	embeddedDir string

	result         ebpftelemetry.BTFResult
	resultMetadata BTFResultMetadata
	loadFunc       funcs.CachedFunc[returnBTF]
	delayedFlusher *time.Timer
}

func initBTFLoader(cfg *Config) *orderedBTFLoader {
	btfLoader := &orderedBTFLoader{
		userBTFPath: cfg.BTFPath,
		embeddedDir: filepath.Join(cfg.BPFDir, "co-re", "btf"),
		result:      ebpftelemetry.BtfNotFound,
	}
	btfLoader.loadFunc = funcs.CacheWithCallback[returnBTF](btfLoader.get, loadKernelSpec.Flush)
	btfLoader.delayedFlusher = time.AfterFunc(btfFlushDelay, btfLoader.Flush)
	return btfLoader
}

type btfLoaderFunc func() (*returnBTF, error)

// Get returns BTF for the running kernel
func (b *orderedBTFLoader) Get() (*returnBTF, ebpftelemetry.COREResult, error) {
	ret, err := b.loadFunc.Do()
	if ret != nil && ret.vmlinux != nil {
		b.delayedFlusher.Reset(btfFlushDelay)
	}
	return ret, ebpftelemetry.COREResult(b.result), err
}

// Flush deletes any cached BTF
func (b *orderedBTFLoader) Flush() {
	b.delayedFlusher.Stop()
	b.loadFunc.Flush()
}

func (b *orderedBTFLoader) get() (*returnBTF, error) {
	b.resultMetadata.numLoadAttempts++

	loaders := []struct {
		result ebpftelemetry.BTFResult
		loader btfLoaderFunc
		desc   string
	}{
		{ebpftelemetry.SuccessCustomBTF, b.loadUser, "configured BTF file"},
		{ebpftelemetry.SuccessDefaultBTF, b.loadKernel, "kernel"},
		{ebpftelemetry.SuccessEmbeddedBTF, b.loadEmbedded, "embedded collection"},
	}
	var err error
	var ret *returnBTF
	for _, l := range loaders {
		log.Debugf("attempting BTF load from %s", l.desc)
		ret, err = l.loader()
		if err != nil {
			err = fmt.Errorf("BTF load from %s: %w", l.desc, err)
			// attempting default kernel when not supported will return this error
			if !errors.Is(err, btf.ErrNotSupported) {
				log.Debugf("error loading BTF: %s", err)
			}
			continue
		}
		if ret != nil {
			log.Debugf("successfully loaded BTF from %s", l.desc)
			b.result = l.result
			b.resultMetadata.loaderUsed = l.desc
			return ret, nil
		}
	}
	return nil, err
}

func (b *orderedBTFLoader) loadKernel() (*returnBTF, error) {
	spec, err := GetKernelSpec()
	if err != nil {
		return nil, err
	}
	b.resultMetadata.filepathUsed = "<unknown, internal to cilium ebpf>"
	return &returnBTF{
		vmlinux:        spec,
		moduleLoadFunc: nil,
	}, nil
}

func (b *orderedBTFLoader) loadUser() (*returnBTF, error) {
	if b.userBTFPath == "" {
		return nil, nil
	}
	spec, err := loadBTFFrom(b.userBTFPath)
	if err != nil {
		return nil, err
	}
	b.resultMetadata.filepathUsed = b.userBTFPath
	return &returnBTF{
		vmlinux:        spec,
		moduleLoadFunc: nil,
	}, nil
}

func (b *orderedBTFLoader) checkForMinimizedBTF(extractDir string) (*returnBTF, error) {
	// <relative_path_in_tarball>/<kernel_version>/<kernel_version>.btf
	btfRelativePath := filepath.Join(extractDir, filepath.Base(extractDir)+".btf")
	extractedBtfPath := filepath.Join(b.embeddedDir, btfRelativePath)
	if _, err := os.Stat(extractedBtfPath); err == nil {
		spec, err := loadBTFFrom(extractedBtfPath)
		if err != nil {
			return nil, err
		}
		b.resultMetadata.filepathUsed = extractedBtfPath
		return &returnBTF{
			vmlinux:        spec,
			moduleLoadFunc: nil,
		}, nil
	}
	return nil, nil
}

func (b *orderedBTFLoader) checkForUnminimizedBTF(extractDir string) (*returnBTF, error) {
	absExtractDir := filepath.Join(b.embeddedDir, extractDir)
	modLoadFunc := func(mod string) (*btf.Spec, error) {
		b.delayedFlusher.Reset(btfFlushDelay)
		return loadBTFFrom(filepath.Join(absExtractDir, mod))
	}
	btfRelativePath := filepath.Join(extractDir, "vmlinux")
	extractedBtfPath := filepath.Join(b.embeddedDir, btfRelativePath)
	if _, err := os.Stat(extractedBtfPath); err == nil {
		spec, err := loadBTFFrom(extractedBtfPath)
		if err != nil {
			return nil, err
		}
		b.resultMetadata.filepathUsed = extractedBtfPath
		return &returnBTF{
			vmlinux:        spec,
			moduleLoadFunc: modLoadFunc,
		}, nil
	}
	return nil, nil
}

func (b *orderedBTFLoader) checkforBTF(extractDir string) (*returnBTF, error) {
	ret, err := b.checkForMinimizedBTF(extractDir)
	if err != nil || ret != nil {
		return ret, err
	}
	ret, err = b.checkForUnminimizedBTF(extractDir)
	if err != nil || ret != nil {
		return ret, err
	}
	return nil, nil
}

func (b *orderedBTFLoader) loadEmbedded() (*returnBTF, error) {
	btfRelativeTarballFilename, err := b.embeddedPath()
	if err != nil {
		return nil, err
	}
	kernelVersion, err := kernel.Release()
	if err != nil {
		return nil, fmt.Errorf("kernel release: %s", err)
	}
	// <relative_path_in_tarball>/<kernel_version>
	extractDir := filepath.Join(filepath.Dir(btfRelativeTarballFilename), kernelVersion)
	absExtractDir := filepath.Join(b.embeddedDir, extractDir)

	// If we've previously extracted the BTF file in question, we can just load it
	ret, err := b.checkforBTF(extractDir)
	if err != nil || ret != nil {
		return ret, err
	}
	log.Debugf("extracted btf file not found at %s: attempting to extract from embedded archive", absExtractDir)

	// The embedded BTFs are compressed twice: the individual BTFs themselves are compressed, and the collection
	// of BTFs as a whole is also compressed.
	// This means that we'll need to first extract the specific BTF which  we're looking for from the collection
	// tarball, and then unarchive it.
	btfTarball := filepath.Join(b.embeddedDir, btfRelativeTarballFilename)
	b.resultMetadata.tarballUsed = btfTarball
	if _, err := os.Stat(btfTarball); errors.Is(err, fs.ErrNotExist) {
		collectionTarball := filepath.Join(b.embeddedDir, btfArchiveName)
		if err := archive.TarXZExtractFile(collectionTarball, btfRelativeTarballFilename, b.embeddedDir); err != nil {
			return nil, fmt.Errorf("extract kernel BTF tarball from collection: %w", err)
		}
	}

	if err := archive.TarXZExtractAll(btfTarball, absExtractDir); err != nil {
		return nil, fmt.Errorf("extract kernel BTF from tarball: %w", err)
	}
	ret, err = b.checkforBTF(extractDir)
	if err != nil || ret != nil {
		return ret, err
	}
	return nil, fmt.Errorf("embedded BTF not found at %s", extractDir)
}

func (b *orderedBTFLoader) embeddedPath() (string, error) {
	platform, err := getBTFPlatform()
	if err != nil {
		return "", fmt.Errorf("BTF platform: %s", err)
	}
	platformVersion, err := kernel.PlatformVersion()
	if err != nil {
		return "", fmt.Errorf("platform version: %s", err)
	}
	kernelVersion, err := kernel.Release()
	if err != nil {
		return "", fmt.Errorf("kernel release: %s", err)
	}
	return b.getEmbeddedBTF(platform, platformVersion, kernelVersion)
}

var kernelVersionPatterns = []struct {
	pattern   *regexp.Regexp
	platforms []btfPlatform
}{
	{regexp.MustCompile(`\.amzn[1-2]\.`), []btfPlatform{platformAmazon}},
	{regexp.MustCompile(`\.el7\.`), []btfPlatform{platformRedhat, platformCentOS, platformOracle}},
	{regexp.MustCompile(`\.el8(_\d)?\.`), []btfPlatform{platformRedhat, platformCentOS, platformOracle}},
	{regexp.MustCompile(`\.el[7-8]uek\.`), []btfPlatform{platformOracle}},
	{regexp.MustCompile(`\.deb10\.`), []btfPlatform{platformDebian}},
	{regexp.MustCompile(`\.fc\d{2}\.`), []btfPlatform{platformFedora}},
	{regexp.MustCompile(`-lp15\d\.`), []btfPlatform{platformOpenSUSELeap}},
	{regexp.MustCompile(`-150300\.`), []btfPlatform{platformOpenSUSELeap}},
}

var errIncorrectOSReleaseMount = errors.New("please mount the /etc/os-release file as /host/etc/os-release in the system-probe container to resolve this")

// getEmbeddedBTF returns the relative path to the BTF *tarball* file
func (b *orderedBTFLoader) getEmbeddedBTF(platform btfPlatform, platformVersion, kernelVersion string) (string, error) {
	btfTarball := kernelVersion + ".btf.tar.xz"
	possiblePaths := b.searchEmbeddedCollection(btfTarball)
	if len(possiblePaths) == 0 {
		return "", fmt.Errorf("no BTF file in embedded collection matching kernel version `%s`", kernelVersion)
	}

	btfRelativePath := filepath.Join(platform.String(), btfTarball)
	if platform == platformUbuntu {
		// Ubuntu BTFs are stored in subdirectories corresponding to platform version.
		// This is because we have BTFs for different versions of ubuntu with the exact same
		// kernel name, so kernel name alone is not a unique identifier.
		btfRelativePath = filepath.Join(platform.String(), platformVersion, btfTarball)
	}
	if slices.Contains(possiblePaths, btfRelativePath) {
		return btfRelativePath, nil
	}
	for i, p := range possiblePaths {
		log.Debugf("possible embedded BTF file path: `%s` (%d of %d)", p, i+1, len(possiblePaths))
	}

	// platform may be incorrectly detected if /etc/os-release is not mounted into the system-probe container
	// try several ways to automatically find correct BTF file

	// if we found a unique file within the collection, use that
	if len(possiblePaths) == 1 {
		pathParts := strings.Split(possiblePaths[0], string(os.PathSeparator))
		if len(pathParts) > 0 {
			if pathParts[0] != platform.String() {
				log.Warnf("BTF platform incorrectly detected as `%s`, using `%s` instead. Mount the /etc/os-release file as /host/etc/os-release in the system-probe container to resolve this warning", platform, pathParts[0])
			}
			if pathParts[0] == platformUbuntu.String() && len(pathParts) > 2 && pathParts[1] != platformVersion {
				log.Warnf("ubuntu platform version incorrectly detected as `%s`, using `%s` instead. Mount the /etc/os-release file as /host/etc/os-release in the system-probe container to resolve this warning", platformVersion, pathParts[1])
			}
			return possiblePaths[0], nil
		}
	}

	// multiple possible paths
	// try a strong association based on kernel version patterns
	for _, kvp := range kernelVersionPatterns {
		if kvp.pattern.MatchString(kernelVersion) {
			// remove possible paths that do not match possible platforms
			possiblePaths = slices.DeleteFunc(possiblePaths, func(s string) bool {
				pform := strings.Split(s, string(os.PathSeparator))[0]
				btfp, err := btfPlatformFromString(pform)
				if err != nil {
					return true
				}
				return !slices.Contains(kvp.platforms, btfp)
			})
			if len(possiblePaths) == 1 {
				// eliminated down to one matching
				return possiblePaths[0], nil
			}
			break
		}
	}

	// still unsure between multiple possible paths, log all unique platforms
	possiblePlatforms := make(map[btfPlatform]bool)
	for _, p := range possiblePaths {
		pform := strings.Split(p, string(os.PathSeparator))[0]
		if btfp, err := btfPlatformFromString(pform); err == nil {
			possiblePlatforms[btfp] = true
		}
	}
	// handle case where ubuntu is actually correct
	if len(possiblePlatforms) == 1 && platform == platformUbuntu && possiblePlatforms[platform] {
		return "", fmt.Errorf("ubuntu platform version incorrectly detected as `%s`. %w", platformVersion, errIncorrectOSReleaseMount)
	}
	platformStrings := make([]string, 0, len(possiblePlatforms))
	for pp := range possiblePlatforms {
		platformStrings = append(platformStrings, pp.String())
	}
	return "", fmt.Errorf("BTF platform incorrectly detected as `%s`. It is likely one of `%s`, but we are unable to automatically decide. %w", platform, strings.Join(platformStrings, ","), errIncorrectOSReleaseMount)
}

func (b *orderedBTFLoader) searchEmbeddedCollection(filename string) []string {
	var matchingPaths []string
	collectionTarball := filepath.Join(b.embeddedDir, btfArchiveName)
	// ignore error because we only care if there are matching paths
	_ = archive.WalkTarXZArchive(collectionTarball, func(_ *tar.Reader, hdr *tar.Header) error {
		if hdr.Typeflag == tar.TypeReg {
			if filepath.Base(hdr.Name) == filename {
				pform := strings.Split(hdr.Name, string(os.PathSeparator))[0]
				// must be a recognized platform
				if _, err := btfPlatformFromString(pform); err == nil {
					matchingPaths = append(matchingPaths, hdr.Name)
				}
			}
		}
		return nil
	})
	return matchingPaths
}

var getBTFPlatform = funcs.Memoize(func() (btfPlatform, error) {
	platform, err := kernel.Platform()
	if err != nil {
		return "", fmt.Errorf("kernel platform: %s", err)
	}
	return btfPlatformFromString(platform)
})

func loadBTFFrom(path string) (*btf.Spec, error) {
	data, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer data.Close()

	return btf.LoadSpecFromReader(data)
}

var loadKernelSpec = funcs.CacheWithCallback[btf.Spec](btf.LoadKernelSpec, btf.FlushKernelSpec)

// GetKernelSpec returns a possibly cached version of the running kernel BTF spec
// it's very important that the caller of this function does not modify the returned value
func GetKernelSpec() (*btf.Spec, error) {
	return loadKernelSpec.Do()
}
