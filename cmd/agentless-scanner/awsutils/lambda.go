// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package awsutils

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/smithy-go"
)

const (
	maxLambdaUncompressed = 256 * 1024 * 1024
)

// SetupLambda downloads and extracts the code of a lambda function.
func SetupLambda(ctx context.Context, scan *types.ScanTask) (string, error) {
	lambdaDir := scan.Path()
	if err := os.MkdirAll(lambdaDir, 0700); err != nil {
		return "", err
	}
	return downloadAndUnzipLambda(ctx, scan, lambdaDir)
}

func downloadAndUnzipLambda(ctx context.Context, scan *types.ScanTask, lambdaDir string) (codePath string, err error) {
	if err := statsd.Count("datadog.agentless_scanner.functions.started", 1.0, scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	defer func() {
		if err != nil {
			var isResourceNotFoundError bool
			var aerr smithy.APIError
			if errors.As(err, &aerr) && aerr.ErrorCode() == "ResourceNotFoundException" {
				isResourceNotFoundError = true
			}
			var tags []string
			if isResourceNotFoundError {
				tags = scan.TagsNotFound()
			} else {
				tags = scan.TagsFailure(err)
			}
			if err := statsd.Count("datadog.agentless_scanner.functions.finished", 1.0, tags, 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		} else {
			if err := statsd.Count("datadog.agentless_scanner.functions.finished", 1.0, scan.TagsSuccess(), 1.0); err != nil {
				log.Warnf("failed to send metric: %v", err)
			}
		}
	}()

	cfg := GetConfigFromCloudID(ctx, scan.Roles, scan.CloudID)
	lambdaclient := lambda.NewFromConfig(cfg)
	if err != nil {
		return "", err
	}

	lambdaFunc, err := lambdaclient.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: aws.String(scan.CloudID.AsText()),
	})
	if err != nil {
		return "", err
	}

	if lambdaFunc.Code.ImageUri != nil {
		return "", fmt.Errorf("lambda: OCI images are not supported")
	}
	if lambdaFunc.Code.Location == nil {
		return "", fmt.Errorf("lambda: no code location")
	}

	archivePath := filepath.Join(lambdaDir, "code.zip")
	log.Tracef("%s: creating file %q", scan, archivePath)
	archiveFile, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer archiveFile.Close()

	lambdaURL := *lambdaFunc.Code.Location
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lambdaURL, nil)
	if err != nil {
		return "", err
	}

	log.Tracef("%s: downloading code", scan)
	resp, err := cfg.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lambda: bad status: %s", resp.Status)
	}

	log.Tracef("%s: copying code archive to %q", scan, archivePath)
	compressedSize, err := io.Copy(archiveFile, resp.Body)
	if err != nil {
		return "", err
	}

	codePath = filepath.Join(lambdaDir, "code")
	err = os.Mkdir(codePath, 0700)
	if err != nil {
		return "", err
	}

	log.Tracef("%s: extracting code in %q", scan, codePath)
	uncompressedSize, err := extractLambdaZip(ctx, archivePath, codePath)
	if err != nil {
		return "", err
	}

	log.Debugf("%s: function retrieved successfully (took %s)", scan, time.Since(scan.StartedAt))
	if err := statsd.Histogram("datadog.agentless_scanner.functions.duration", float64(time.Since(scan.StartedAt).Milliseconds()), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Histogram("datadog.agentless_scanner.functions.size_compressed", float64(compressedSize), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}
	if err := statsd.Histogram("datadog.agentless_scanner.functions.size_uncompressed", float64(uncompressedSize), scan.Tags(), 1.0); err != nil {
		log.Warnf("failed to send metric: %v", err)
	}

	log.Debugf("%s: downloaded and extracted code ; compressed_size=%d uncompressed_size=%d", scan, compressedSize, uncompressedSize)
	return codePath, nil
}

func extractLambdaZip(ctx context.Context, zipPath, destinationPath string) (uint64, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("extractLambdaZip: openreader: %w", err)
	}
	defer r.Close()

	var uncompressed uint64
	for _, f := range r.File {
		if ctx.Err() != nil {
			return uncompressed, ctx.Err()
		}
		name := filepath.Join("/", f.Name)[1:]
		dest := filepath.Join(destinationPath, name)
		destDir := filepath.Dir(dest)
		if err := os.MkdirAll(destDir, 0700); err != nil {
			return uncompressed, err
		}
		if strings.HasSuffix(f.Name, "/") {
			if err := os.Mkdir(dest, 0700); err != nil {
				return uncompressed, err
			}
		} else {
			reader, err := f.Open()
			if err != nil {
				return uncompressed, fmt.Errorf("extractLambdaZip: open: %w", err)
			}
			defer reader.Close()
			writer, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return uncompressed, fmt.Errorf("extractLambdaZip: write: %w", err)
			}
			defer writer.Close()
			if uncompressed+f.UncompressedSize64 > maxLambdaUncompressed {
				return uncompressed, fmt.Errorf("extractLambdaZip: uncompressed size is too big")
			}
			n, err := io.Copy(writer, reader)
			uncompressed += uint64(n)
			if err != nil {
				return uncompressed, fmt.Errorf("extractLambdaZip: copy: %w", err)
			}
		}
	}
	return uncompressed, nil
}
