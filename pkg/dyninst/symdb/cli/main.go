// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// CLI for generating SymDB data from binaries.
package main

import (
	"archive/tar"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	container "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	binaryPathFlag = flag.String("binary-path", "",
		"Path to the binary to analyze. If -image  is specified, the path is looked up "+
			"inside the image. If -image is not specified, it defaults to /usr/local/bin/<base image name>.")
	imageName = flag.String("image", "",
		"Container image to extract and analyze. If not specified, the binary path must be a local file. "+
			"Can be <image name>:<tag>; otherwise the \"latest\" tag is used.")
	platform = flag.String("platform", "",
		"Platform for the container image (e.g. linux/amd64, linux/arm64).")

	onlyFirstParty = flag.Bool("only-1stparty", false,
		"Only output symbols for \"1st party\" code (i.e. code from modules belonging "+
			"to the same GitHub org as the main one).")

	silent = flag.Bool("silent", false, "If set, the collected symbols are not printed.")

	upload     = flag.Bool("upload", false, "If specified, the SymDB data will be uploaded through a trace-agent.")
	noopUpload = flag.Bool("noop-upload", false,
		"If true, exercise the upload code path (marshal JSON + gzip every "+
			"batch) without actually performing the HTTP request. Useful for "+
			"profiling the symdb flow end-to-end. Implies -silent. Mutually "+
			"exclusive with -upload.")
	uploadSite = flag.String("upload-site", "", "The site to which SymDB data will be uploaded. "+
		"If neither -upload-site or -upload-url are specified, datad0g.com is used as the site.")
	uploadURL = flag.String("upload-url",
		"https://debugger-intake.datad0g.com/api/v2/debugger",
		"If specified, the SymDB data will be uploaded to this URL. "+
			"Either -upload-site or -upload-url must be set when -upload is specified.")
	uploadService = flag.String("service", "", "The service name to use when uploading SymDB data.")
	uploadVersion = flag.String("version", "", "The version to use when uploading SymDB data.")
	uploadAPIKey  = flag.String("api-key", "", "The API key used to authenticate uploads.")

	// Knobs that mirror the system-probe symdbManager so that running the
	// CLI is representative of the in-module behaviour for memory profiling.
	flushThresholdBytes = flag.Int("flush-threshold-bytes", uploader.DefaultFlushThresholdBytes,
		"Compressed-size threshold (in bytes) at which an upload batch is "+
			"flushed as an HTTP request. Matches the system-probe symdbManager "+
			"default.")
	agentVersionFlag = flag.String("agent-version", version.AgentVersion,
		"The agent version string to embed in uploaded scopes. "+
			"Defaults to the build-time agent version (matches the system-probe).")
	diskCacheEnabled = flag.Bool("disk-cache", true,
		"If true, use an on-disk DiskCache for loading object files (matching "+
			"the system-probe DiskCacheEnabled path). If false, an in-memory "+
			"loader is used.")
	diskCacheDir = flag.String("disk-cache-dir", "",
		"Directory for the on-disk DiskCache. If unset, a temporary directory "+
			"is created and removed on exit. Only used when -disk-cache is true.")
	diskCacheMaxBytes = flag.Uint64("disk-cache-max-bytes", 2<<30, /* 2 GiB */
		"Maximum aggregate size of cached decompressed sections, in bytes. "+
			"Default matches the system-probe (2 GiB). Only used when -disk-cache is true.")
	diskCacheReservedBytes = flag.Uint64("disk-cache-reserved-bytes", 512<<20, /* 512 MiB */
		"Minimum free disk space, in bytes, that must remain after writing "+
			"a section. Default matches the system-probe (512 MiB). "+
			"Only used when -disk-cache is true.")
	diskCacheReservedPercent = flag.Float64("disk-cache-reserved-percent", 0,
		"Minimum free disk space, as a percentage, that must remain after "+
			"writing a section. Only used when -disk-cache is true.")

	pprofPort  = flag.Int("pprof-port", 8081, "Port for pprof server.")
	traceFile  = flag.String("trace", "", "Path to the file to save a runtime/trace execution trace to.")
	cpuProfile = flag.String("cpuprofile", "", "Path to the file to save a CPU profile to.")
	memProfile = flag.String("memprofile", "",
		"Where to write heap profile(s). If this is a path ending in `/` or "+
			"an existing directory, a heap profile is written every "+
			"-memprofile-interval into that directory as `heap-<seq>.pprof`, "+
			"plus `heap-final.pprof` at exit. Otherwise, treated as a single "+
			"file path written at exit only.")
	memProfileInterval = flag.Duration("memprofile-interval", 10*time.Millisecond,
		"Interval between periodic heap profile dumps when -memprofile is a directory.")
	memProfRate = flag.Int("memprofilerate", 0,
		"If non-zero, sets runtime.MemProfileRate before symbol extraction.")
)

func main() {
	flag.Parse()
	if *binaryPathFlag == "" && *imageName == "" {
		fmt.Print(`Usage: symdbcli [-image <container image name>[:image tag]] [-binary-path <path-to-binary>] [-only-1stparty] [-silent]

The symbols from the specified container image (-image) or binary (-binary-path)
will be extracted and either printed to stdout or uploaded to the backend.

To upload the SymDB data rather than printing it, use:
-upload -service <service> -version <version> -api-key <api-key> [-upload-site <site>]

`)
		os.Exit(1)
	}

	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	log.SetupLogger(log.Default(), logLevel)
	defer log.Flush()

	// Start the pprof server.
	go func() {
		_ = http.ListenAndServe(fmt.Sprintf("localhost:%d", *pprofPort), nil)
	}()

	if err := run(); err != nil {
		log.Errorf("Error: %v", err)
		log.Flush()
		os.Exit(1)
	}
}

func run() (retErr error) {
	var localBinPath string
	if *imageName == "" {
		// No image specified: treat binaryPathFlag as a local file
		if *binaryPathFlag == "" {
			return errors.New("-binary-path is required when -image is not specified")
		}
		info, err := os.Stat(*binaryPathFlag)
		if err != nil {
			return fmt.Errorf("binary path %q does not exist: %w", *binaryPathFlag, err)
		}
		if info.IsDir() {
			return fmt.Errorf("-binary-path %q is a directory, expected a file", *binaryPathFlag)
		}
		localBinPath = *binaryPathFlag
	} else {
		imageRef := *imageName
		if !strings.ContainsRune(imageRef, '/') {
			imageRef = "registry.ddbuild.io/" + imageRef
		}
		binPath := *binaryPathFlag
		// If no binary path is specified, default to /usr/local/bin/<image name>.
		if binPath == "" {
			// Parse out the image name, ignoring the registry and version.
			ref, err := name.ParseReference(imageRef)
			if err != nil {
				return fmt.Errorf("could not parse image reference %q: %w", imageRef, err)
			}
			repo := ref.Context().RepositoryStr()
			binPath = "/usr/local/bin/" + repo
			log.Infof("No -binary-path specified, defaulting to %q", binPath)
		}
		var err error
		localBinPath, err = extractBinaryFromImage(imageRef, binPath)
		if err != nil {
			msg := err.Error()
			if *binaryPathFlag == "" {
				msg += "\nFile not found at default location. Please specify -binary-path to override."
			}
			return errors.New(msg)
		}
		defer os.Remove(localBinPath)
		log.Infof("Extracted binary: %s", localBinPath)
	}

	log.Infof("Analyzing binary: %s", localBinPath)
	scope := symdb.ExtractScopeAllSymbols

	if *upload && *noopUpload {
		return errors.New("-upload and -noop-upload are mutually exclusive")
	}

	var uploadURLParsed *url.URL
	if *upload {
		// Upload implies silent mode.
		*silent = true

		if *uploadURL != "" && *uploadSite != "" {
			return errors.New("only one of -upload-url or -upload-side must be specified")
		}
		if *uploadSite == "" {
			*uploadSite = "datad0g.com"
		}
		if *uploadURL == "" {
			*uploadURL = fmt.Sprintf("https://debugger-intake.%s/api/v2/debugger", *uploadSite)
		}

		if *uploadAPIKey == "" {
			return errors.New("-api-key must be specified when -upload is used")
		}
		var err error

		uploadURLParsed, err = url.Parse(*uploadURL)
		if err != nil {
			return fmt.Errorf("failed to parse upload URL %s: %w", *uploadURL, err)
		}

		if *uploadService == "" || *uploadVersion == "" {
			return errors.New("when --upload is specified, --service and --version must also be specified")
		}
	}
	if *noopUpload {
		// Noop-upload implies silent mode (we don't print symbols when
		// exercising the upload path, just like a real upload).
		*silent = true
	}

	if *onlyFirstParty {
		log.Infof("Extracting only 1st party symbols")
		scope = symdb.ExtractScopeModulesFromSameOrg
	}

	if *memProfRate > 0 {
		runtime.MemProfileRate = *memProfRate
	}

	// If -memprofile is a directory, start a goroutine that dumps a heap
	// profile every -memprofile-interval. The final dump is written below,
	// after extraction completes.
	memProfileIsDir := false
	var memProfileDir, memProfileFile string
	if *memProfile != "" {
		mp := *memProfile
		if strings.HasSuffix(mp, string(os.PathSeparator)) {
			memProfileIsDir = true
			memProfileDir = strings.TrimSuffix(mp, string(os.PathSeparator))
		} else if info, err := os.Stat(mp); err == nil && info.IsDir() {
			memProfileIsDir = true
			memProfileDir = mp
		} else {
			memProfileFile = mp
		}
	}
	if memProfileIsDir {
		if err := os.MkdirAll(memProfileDir, 0o755); err != nil {
			return fmt.Errorf("failed to create memprofile dir %s: %w", memProfileDir, err)
		}
		log.Infof("Writing periodic heap profiles to %s every %s", memProfileDir, *memProfileInterval)
		stopHeapDumper := make(chan struct{})
		heapDumperDone := make(chan struct{})
		go func() {
			defer close(heapDumperDone)
			ticker := time.NewTicker(*memProfileInterval)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeapDumper:
					return
				case t := <-ticker.C:
					path := filepath.Join(memProfileDir, fmt.Sprintf("heap-%s.pprof", t.UTC().Format("2006-01-02T15-04-05.000000000Z")))
					if err := writeHeapProfile(path); err != nil {
						log.Warnf("failed to write periodic heap profile %s: %v", path, err)
					}
				}
			}
		}()
		defer func() {
			close(stopHeapDumper)
			<-heapDumperDone
			final := filepath.Join(memProfileDir, fmt.Sprintf("heap-final-%s.pprof", time.Now().UTC().Format("2006-01-02T15-04-05.000000000Z")))
			if err := writeHeapProfile(final); err != nil && retErr == nil {
				retErr = fmt.Errorf("failed to write final heap profile: %w", err)
			}
		}()
	}

	// Start the CPU profile, if requested. Started before the runtime trace
	// so that StopCPUProfile() runs after Stop() of the trace.
	if *cpuProfile != "" {
		log.Infof("Writing CPU profile to %s", *cpuProfile)
		f, err := os.Create(*cpuProfile)
		if err != nil {
			return fmt.Errorf("failed to create CPU profile file %s: %w", *cpuProfile, err)
		}
		defer func() {
			if err := f.Close(); err != nil && retErr == nil {
				retErr = fmt.Errorf("failed to close CPU profile file: %w", err)
			}
		}()
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("failed to start CPU profile: %w", err)
		}
		defer pprof.StopCPUProfile()
	}

	// Start tracing if we were asked to.
	if *traceFile != "" {
		log.Infof("Tracing symbol extraction to %s", *traceFile)
		f, err := os.Create(*traceFile)
		if err != nil {
			return fmt.Errorf("failed to open trace file %s: %w", *traceFile, err)
		}
		defer func() {
			if err := f.Close(); err != nil && retErr == nil {
				retErr = fmt.Errorf("failed to close trace file: %w", err)
			}
		}()
		if err := trace.Start(f); err != nil {
			return fmt.Errorf("failed to start trace: %w", err)
		}
		defer trace.Stop()
	}

	// Build the object loader. When -disk-cache is set (the default), mirror
	// the system-probe DiskCacheEnabled code path: pass the *object.DiskCache
	// as the loader and via ExtractOptions.DiskCache so that symdb uses the
	// on-disk generic-function index.
	var objectLoader object.Loader
	var diskCache *object.DiskCache
	extractOpts := symdb.ExtractOptions{Scope: scope}
	if *diskCacheEnabled {
		cacheDir := *diskCacheDir
		if cacheDir == "" {
			d, err := os.MkdirTemp("", "symdb-cli-cache-*")
			if err != nil {
				return fmt.Errorf("failed to create temp disk cache dir: %w", err)
			}
			cacheDir = d
			defer func() {
				if err := os.RemoveAll(cacheDir); err != nil {
					log.Warnf("failed to remove temp disk cache dir %s: %v", cacheDir, err)
				}
			}()
		} else if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return fmt.Errorf("failed to create disk cache dir %s: %w", cacheDir, err)
		}
		log.Infof("Using on-disk object cache at %s (max %d bytes)", cacheDir, *diskCacheMaxBytes)
		dc, err := object.NewDiskCache(object.DiskCacheConfig{
			DirPath:                  cacheDir,
			MaxTotalBytes:            *diskCacheMaxBytes,
			RequiredDiskSpaceBytes:   *diskCacheReservedBytes,
			RequiredDiskSpacePercent: *diskCacheReservedPercent,
		})
		if err != nil {
			return fmt.Errorf("failed to create disk cache: %w", err)
		}
		objectLoader = dc
		diskCache = dc
		extractOpts.DiskCache = dc
	} else {
		objectLoader = object.NewInMemoryLoader()
	}

	start := time.Now()

	it, err := symdb.PackagesIterator(localBinPath, objectLoader, extractOpts)
	if err != nil {
		return err
	}

	// Build the per-yield encoder. Three modes:
	//  - -upload: real BatchEncoder shipping to a SymDB intake.
	//  - -noop-upload: real BatchEncoder shipping to an in-process
	//    httptest.Server whose handler reads each request body to EOF and
	//    discards it. This exercises the entire upload pipeline (JSON +
	//    gzip + multipart + HTTP send + connection lifecycle) without
	//    talking to a real intake.
	//  - neither: printingEncoder that prints each package via
	//    symdbprinter (or just counts, when -silent is set).
	var enc uploader.PackageEncoder
	switch {
	case *upload, *noopUpload:
		var (
			intakeURL string
			service   = *uploadService
			version   = *uploadVersion
			headers   [][2]string
		)
		if *upload {
			// Headers to attach to every HTTP request. When the
			// system-probe does the uploading, it sends the data through
			// the local trace-agent, which deals with setting these
			// headers. The CLI uploads directly so it must set them
			// itself.
			intakeURL = uploadURLParsed.String()
			headers = [][2]string{
				{"DD-EVP-ORIGIN", "symdb-cli"},
				{"DD-EVP-ORIGIN-VERSION", "0.1"},
				{"DD-API-KEY", *uploadAPIKey},
			}
		} else {
			// -noop-upload: the BatchEncoder still expects a real HTTP
			// endpoint to POST batches to, so we stand up an in-process
			// server that drains and discards each request body. This
			// exercises the full upload pipeline (marshal, gzip, multipart,
			// HTTP send) end-to-end without needing a real intake.
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()
			intakeURL = srv.URL
			service = "noop-service"
			version = "noop-version"
			log.Infof("Exercising upload pipeline against in-process server %s.", srv.URL)
		}
		runtimeID := fmt.Sprintf("manual-upload-%d", rand.Intn(1000))
		realEnc, err := uploader.NewBatchEncoder(
			intakeURL, service, version, runtimeID,
			uuid.New(), diskCache, headers,
		)
		if err != nil {
			return fmt.Errorf("failed to create batch encoder: %w", err)
		}
		defer func() { _ = realEnc.Close() }()
		enc = realEnc
	default:
		enc = &printingEncoder{out: os.Stdout, silent: *silent}
	}

	stats, err := uploader.RunUploadLoop(
		context.Background(), enc, it, *agentVersionFlag, *flushThresholdBytes,
	)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	if *upload || *noopUpload {
		log.Infof("SymDB: Successfully uploaded symbols: %d packages, %d functions, %d chunks",
			stats.Packages, stats.Functions, stats.Batches)
	}

	log.Infof("Symbol extraction completed in %s.", time.Since(start))
	log.Infof("Symbol statistics for %s: Packages: %d, Functions: %d",
		localBinPath, stats.Packages, stats.Functions)

	if *silent && !*upload && !*noopUpload {
		log.Infof("--silent specified; symbols not serialized.")
	}

	if memProfileFile != "" {
		log.Infof("Writing heap profile to %s", memProfileFile)
		runtime.GC()
		if err := writeHeapProfile(memProfileFile); err != nil {
			return err
		}
	}

	return nil
}

// writeHeapProfile writes the current heap profile to path. Does not run a
// GC first; the caller is responsible for triggering a GC if it wants the
// profile to reflect post-GC state.
func writeHeapProfile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create heap profile file %s: %w", path, err)
	}
	defer f.Close()
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		return fmt.Errorf("failed to write heap profile %s: %w", path, err)
	}
	return nil
}

// printingEncoder satisfies uploader.PackageEncoder by rendering each
// AddPackage call as text to out via symdbprinter. Size always returns 0 and
// Flush is a no-op, so RunUploadLoop never tries to flush a batch.
type printingEncoder struct {
	out    io.Writer
	silent bool
}

func (p *printingEncoder) AddPackage(pkg symdb.Package, agentVersion string) error {
	if p.silent {
		return nil
	}
	scope := uploader.ConvertPackageToScope(pkg, agentVersion)
	if err := symdbprinter.SerializeScope(p.out, scope); err != nil {
		return fmt.Errorf("failed to serialize package: %w", err)
	}
	return nil
}

func (p *printingEncoder) Size() int                         { return 0 }
func (p *printingEncoder) Flush(context.Context, bool) error { return nil }
func (p *printingEncoder) Close() error                      { return nil }

// extractBinaryFromImage extracts a binary from an image and returns the path
// to the extracted file.
func extractBinaryFromImage(imageRef string, binaryPath string) (string, error) {
	log.Infof("Pulling image: %s...", imageRef)

	// Pull the image.
	var opts []crane.Option
	if *platform != "" {
		p, err := container.ParsePlatform(*platform)
		if err != nil {
			return "", fmt.Errorf("failed to parse platform: %w", err)
		}
		opts = append(opts, crane.WithPlatform(p))
	}
	img, err := crane.Pull(imageRef, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to pull image %s: %w", imageRef, err)
	}
	log.Infof("Pulling image: %s... done", imageRef)

	// Unpack the image into a temp dir as a tarball, then untar it.
	tarFile, err := os.CreateTemp(
		"",
		fmt.Sprintf("img-%s-*.tar", strings.ReplaceAll(url.PathEscape(*imageName), "/", "-")),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create temp tar file: %w", err)
	}
	defer os.Remove(tarFile.Name())
	defer tarFile.Close()

	log.Infof("Extracting image: %s...", imageRef)
	if err := crane.Export(img, tarFile); err != nil {
		return "", fmt.Errorf("failed to export image to tar: %w", err)
	}
	log.Infof("Extracting image: %s... done", imageRef)
	if _, err := tarFile.Seek(0, 0); err != nil {
		return "", fmt.Errorf("failed to rewind tar file: %w", err)
	}

	binPath := filepath.Join(os.TempDir(), path.Base(binaryPath))
	// Untar the requested binaryPathFlag from the image tar.
	found, err := untarSingleFile(tarFile, binaryPath, binPath)
	if err != nil {
		return "", fmt.Errorf("failed to untar image: %w", err)
	}
	if !found {
		return "", fmt.Errorf("file %q does not exist in image %q", binaryPath, imageRef)
	}
	return binPath, nil
}

// untarSingleFile extracts one file from the tar archive r into outPath.
// Returns false if the requested file does not exist in the archive.
func untarSingleFile(r io.Reader, filePath, outPath string) (bool, error) {
	log.Infof("Untarring %s from image...", filePath)
	// Strip leading slash, if specified. Inside the tar archive, the paths
	// don't start with a slash.
	filePath = strings.TrimPrefix(filePath, "/")
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
		if hdr.Name != filePath {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return false, err
		}
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			return false, err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return false, err
		}
		f.Close()
		log.Infof("Untarring %s from image... done", filePath)
		return true, nil
	}
	return false, nil
}
