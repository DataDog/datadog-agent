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
	_ "net/http/pprof"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime/trace"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	container "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/symdbutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symdb/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	uploadSite = flag.String("upload-site", "", "The site to which SymDB data will be uploaded. "+
		"If neither -upload-site or -upload-url are specified, datad0g.com is used as the site.")
	uploadURL = flag.String("upload-url",
		"https://debugger-intake.datad0g.com/api/v2/debugger",
		"If specified, the SymDB data will be uploaded to this URL. "+
			"Either -upload-site or -upload-url must be set when -upload is specified.")
	uploadService = flag.String("service", "", "The service name to use when uploading SymDB data.")
	uploadVersion = flag.String("version", "", "The version to use when uploading SymDB data.")
	uploadAPIKey  = flag.String("api-key", "", "The API key used to authenticate uploads.")

	pprofPort = flag.Int("pprof-port", 8081, "Port for pprof server.")
	traceFile = flag.String("trace", "", "Path to the file to save an execution trace to.")
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
	start := time.Now()
	scope := symdb.ExtractScopeAllSymbols

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

	if *onlyFirstParty {
		log.Infof("Extracting only 1st party symbols")
		scope = symdb.ExtractScopeModulesFromSameOrg
	}

	// Start tracing if we were asked to.
	tracing := *traceFile != ""
	if tracing {
		log.Infof("Tracing symbol extraction to %s", *traceFile)
		f, err := os.OpenFile(*traceFile, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return fmt.Errorf("failed to open trace file %s: %w", *traceFile, err)
		}
		defer func() {
			_ = f.Close()
		}()
		if err := trace.Start(f); err != nil {
			return fmt.Errorf("failed to start trace: %w", err)
		}
		defer trace.Stop()
	}

	opt := symdb.ExtractOptions{Scope: scope}

	var up *uploader.SymDBUploader
	// Headers to attach to every HTTP request. When the system-probe does the
	// uploading, it sends the data through the local trace-agent, which deals
	// with setting these headers.
	headers := [][2]string{
		{"DD-EVP-ORIGIN", "symdb-cli"},
		{"DD-EVP-ORIGIN-VERSION", "0.1"},
		{"DD-API-KEY", *uploadAPIKey},
	}
	if *upload {
		up = uploader.NewSymDBUploader(
			uploadURLParsed.String(),
			*uploadService, *uploadVersion,
			fmt.Sprintf("manual-upload-%d", rand.Intn(1000)),
			headers...,
		)
	}
	out := symdbutil.MakePanickingWriter(os.Stdout)
	it, err := symdb.PackagesIterator(localBinPath, object.NewInMemoryLoader(), opt)
	if err != nil {
		return err
	}

	uploadBuffer := make([]uploader.Scope, 0, 100)
	bufferFuncs := 0
	// Flush every so ofter in order to not store too many scopes in memory.
	const maxBufferFuncs = 10000
	uploadID := uuid.New()
	batchNum := 0
	maybeFlush := func(final bool) error {
		if len(uploadBuffer) == 0 {
			return nil
		}
		if final || bufferFuncs >= maxBufferFuncs {
			log.Tracef("SymDB: uploading symbols chunk: %d packages, %d functions", len(uploadBuffer), bufferFuncs)
			err := up.UploadBatch(
				context.Background(),
				uploader.UploadInfo{
					UploadID: uploadID,
					BatchNum: batchNum,
					Final:    final,
				},
				uploadBuffer)
			if err != nil {
				return fmt.Errorf("upload failed: %w", err)
			}
			uploadBuffer = uploadBuffer[:0]
			bufferFuncs = 0
		}
		return nil
	}

	var stats symbolStats
	for pkg, err := range it {
		if err != nil {
			return err
		}

		if up != nil {
			scope := uploader.ConvertPackageToScope(pkg.Package, "cli" /* agentVersion */)
			uploadBuffer = append(uploadBuffer, scope)
			bufferFuncs += pkg.Stats().NumFunctions
			if err := maybeFlush(pkg.Final); err != nil {
				return err
			}
		}

		stats.addPackage(pkg.Package)

		if !*silent {
			pkg.Serialize(out)
		}
	}
	log.Info("Upload completed")

	trace.Stop()
	log.Infof("Symbol extraction completed in %s.", time.Since(start))
	log.Infof("Symbol statistics for %s: %s", localBinPath, stats)

	if *silent && !*upload {
		log.Infof("--silent specified; symbols not serialized.")
	}

	return nil
}

type symbolStats struct {
	numPackages  int
	numTypes     int
	numFunctions int
}

func (stats *symbolStats) addPackage(pkg symdb.Package) {
	stats.numPackages++
	s := pkg.Stats()
	stats.numTypes += s.NumTypes
	stats.numFunctions += s.NumFunctions
}

func (stats symbolStats) String() string {
	return fmt.Sprintf("Packages: %d, Types: %d, Functions: %d",
		stats.numPackages, stats.numTypes, stats.numFunctions)
}

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
