# Serverless-Init Release Pipeline

This directory contains the GitLab CI pipeline for building and releasing serverless-init container images.

## 📋 Overview

The pipeline builds serverless-init binaries and container images for both standard (Debian-based) and Alpine Linux variants, supporting both amd64 and arm64 architectures. The `serverless_init_dotnet.sh` file is also copied into images to help users instrument dotnet code.

**Build Registry:**
- Images are built and published to **registry.ddbuild.io** (Internal CI Registry)
  - RC images: `registry.ddbuild.io/ci/datadog-agent/serverless-init-dev`
  - Production images: `registry.ddbuild.io/ci/datadog-agent/serverless-init`

**Public Registries:**
- Images are automatically replicated to **public registries** via downstream pipeline trigger to `DataDog/public-images`
  - ECR: `public.ecr.aws/datadog/serverless-init`
  - GCR: `gcr.io/datadoghq/serverless-init` -> fronted by `registry.datadoghq.com`
  - GAR: `us-docker.pkg.dev/datadoghq/gcr.io/serverless-init`
  - Docker Hub: `docker.io/datadog/serverless-init`


## 🚀 Usage

### Releasing a Release Candidate (RC)

1. Go to [GitLab Pipelines](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines/new)
2. Select the branch you want to release from (e.g., `main` or a feature branch)
3. Choose the pipeline: `.gitlab/serverless-init/release.yml`
4. Set variables:
   - `TAG`: Your RC version (e.g., `1.7.8-rc1`)
   - `LATEST_TAG`: `no`
   - `BUILD_TAGS`: `serverless otlp zlib zstd` (default)
   - `AGENT_VERSION`: (optional) specific agent version, or leave empty for default
5. Run the pipeline

**Results (Build Registry):**
- `registry.ddbuild.io/ci/datadog-agent/serverless-init-dev:1.7.8-rc1`
- `registry.ddbuild.io/ci/datadog-agent/serverless-init-dev:1.7.8-rc1-alpine`

**Automatic Replication (Public Registries):**

The pipeline automatically triggers downstream jobs to replicate the RC images to our internal registry.
- Standard: `serverless-init-dev:1.7.8-rc1`
- Alpine: `serverless-init-dev:1.7.8-rc1-alpine`

### Testing the RC

Use the [serverless-init-self-monitoring GitLab pipeline](https://gitlab.ddbuild.io/DataDog/serverless-init-self-monitoring/-/pipelines/new) to deploy and test your RC:

1. Set `AGENT_IMAGE` to your RC image from either:
   - Build registry: `registry.ddbuild.io/ci/datadog-agent/serverless-init-dev:1.7.8-rc1`
   - Public registry (after replication): `public.ecr.aws/datadog/serverless-init-dev:1.7.8-rc1` or other public registries
2. Set `ENVIRONMENT` to `rc`
3. Run the pipeline
4. Monitor the [self-monitoring dashboard](https://ddserverless.datadoghq.com/dashboard/c73-7ff-zpk/azure-gcp-self-monitoring)

### Releasing a Production Version

Once your RC has been validated:

1. **Create a tag** in the datadog-agent repository:
   ```bash
   git checkout <branch-used-for-rc>
   git tag serverless-init-1.7.8
   git push origin serverless-init-1.7.8
   ```

2. Go to [GitLab Pipelines](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/pipelines/new)
3. Select the tag you just created: `serverless-init-1.7.8`
4. Choose the pipeline: `.gitlab/serverless-init-release.yml`
5. Set variables:
   - `TAG`: Your production version (e.g., `1.7.8`)
   - `LATEST_TAG`: `yes`
   - `BUILD_TAGS`: `serverless otlp zlib zstd` (default)
   - `AGENT_VERSION`: (optional) specific agent version
6. Run the pipeline

**Results (Build Registry):**
- `registry.ddbuild.io/ci/datadog-agent/serverless-init:1.7.8`
- `registry.ddbuild.io/ci/datadog-agent/serverless-init:1.7.8-alpine`
- `registry.ddbuild.io/ci/datadog-agent/serverless-init:latest`
- `registry.ddbuild.io/ci/datadog-agent/serverless-init:latest-alpine`

**Automatic Replication (Public Registries):**

The pipeline automatically triggers **4 downstream jobs** in the `DataDog/public-images` project to replicate images to public registries (ECR, GCR, GAR, Docker Hub):

- **Standard variant**: `serverless-init:1.7.8`, `serverless-init:latest`, `serverless-init:1`
- **Alpine variant**: `serverless-init:1.7.8-alpine`, `serverless-init:latest-alpine`, `serverless-init:1-alpine`

This replication happens **automatically** - you don't need to manually trigger anything. The pipeline triggers separate jobs for standard and alpine variants.

## 🏗️ Pipeline Architecture

The pipeline consists of two stages:

### Stage 1: Build and Publish (runs in parallel)
- `build-and-publish-standard`: Builds and publishes the standard (Debian) variant
- `build-and-publish-alpine`: Builds and publishes the Alpine variant

Each job:
1. Builds the Go binary for amd64 and arm64 (using Docker buildx cross-compilation)
2. Creates the final multi-arch container image with proper OCI labels
3. Pushes to `registry.ddbuild.io`

### Stage 2: Trigger Registry (2 jobs at a time, 4 jobs total)
**Production images** (runs when `LATEST_TAG=yes`):
- `trigger-registry-replication-prod-standard`: Replicates standard variant
- `trigger-registry-replication-prod-alpine`: Replicates alpine variant

**RC images** (runs when `LATEST_TAG=no`):
- `trigger-registry-replication-rc-standard`: Replicates standard variant to `-dev` tags
- `trigger-registry-replication-rc-alpine`: Replicates alpine variant to `-dev` tags

All jobs trigger the `DataDog/public-images` project which replicates to the datadog registry, ECR, GCR, GAR, and Docker Hub.

## 🔍 Troubleshooting

### Authentication Issues

If you encounter authentication errors:

1. Verify you have access to `registry.ddbuild.io` (should be automatic for GitLab runners)
2. Check that the `.login_to_docker_readonly` reference is working correctly
3. For replication, ensure the downstream pipeline trigger has proper permissions to the `DataDog/public-images` project

### Build Failures

- Check that the `TAG` variable is set and follows semantic versioning
- Verify `BUILD_TAGS` includes required Go build tags: `serverless otlp zlib zstd`
- Ensure the datadog-agent source code is in a buildable state

### Replication Failures

If the automatic replication to public registries fails:
- Check the downstream pipelines in `DataDog/public-images` for errors
- Verify that the images exist in `registry.ddbuild.io` before replication
- Ensure all 4 trigger jobs (2 for standard, 2 for alpine) completed successfully
- Check that the `IMG_SOURCES` variables point to the correct image tags

## 📚 Related Documentation

- [Releasing a new version](https://datadoghq.atlassian.net/wiki/spaces/SLS/pages/3048800938/Releasing+a+new+version) (Confluence)
- [serverless-init-self-monitoring](https://github.com/DataDog/serverless-init-self-monitoring)
- [public-images pipeline](https://gitlab.ddbuild.io/DataDog/public-images)
