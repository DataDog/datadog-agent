# Serverless-Init Release Pipeline

This directory contains the GitLab CI pipeline for building and releasing serverless-init container images.

## 📋 Overview

The pipeline builds serverless-init binaries and container images for both standard (Debian-based) and Alpine Linux variants, supporting both amd64 and arm64 architectures. Images are published to:

- **registry.datadoghq.com** (Internal Datadog Registry): `registry.datadoghq.com/serverless-init` (prod) or `registry.datadoghq.com/serverless-init-dev` (RC)

The `serverless_init_dotnet.sh` file is also copied into images to help users instrument dotnet code.

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

**Results:**
- `registry.datadoghq.com/serverless-init-dev:1.7.8-rc1`
- `registry.datadoghq.com/serverless-init-dev:1.7.8-rc1-alpine`

### Testing the RC

Use the [serverless-init-self-monitoring GitLab pipeline](https://gitlab.ddbuild.io/DataDog/serverless-init-self-monitoring/-/pipelines/new) to deploy and test your RC:

1. Set `AGENT_IMAGE` to your RC image (e.g., `registry.datadoghq.com/serverless-init-dev:1.7.8-rc1`)
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

**Results:**
- `registry.datadoghq.com/serverless-init:1.7.8`
- `registry.datadoghq.com/serverless-init:1.7.8-alpine`
- `registry.datadoghq.com/serverless-init:latest`
- `registry.datadoghq.com/serverless-init:latest-alpine`

### Replicating to Public Registries (ECR, GCR, GAR, Docker Hub)

After releasing to production, replicate images to public registries using the [public-images pipeline](https://gitlab.ddbuild.io/DataDog/public-images/-/pipelines/new).

Run **twice** (once for standard, once for alpine):

**Standard variant:**
- `IMG_SOURCES`: `registry.datadoghq.com/serverless-init:1.7.8`
- `IMG_DESTINATIONS`: `serverless-init:1.7.8,serverless-init:latest,serverless-init:1`
- `IMG_SIGNING`: `false`

**Alpine variant:**
- `IMG_SOURCES`: `registry.datadoghq.com/serverless-init:1.7.8-alpine`
- `IMG_DESTINATIONS`: `serverless-init:1.7.8-alpine,serverless-init:latest-alpine,serverless-init:1-alpine`
- `IMG_SIGNING`: `false`

This replicates images to ECR, GCR, GAR, and Docker Hub.

## 🏗️ Pipeline Architecture

The pipeline runs **2 parallel jobs** that each:
1. Build the Go binary for amd64 and arm64 (using Docker buildx cross-compilation)
2. Create the final multi-arch container image
3. Push to registry.datadoghq.com

**Jobs:**
- `build-and-publish-standard`: Builds and publishes the standard (Debian) variant
- `build-and-publish-alpine`: Builds and publishes the Alpine variant

## 🔍 Troubleshooting

### Authentication Issues

If you encounter authentication errors:

1. Verify GitLab CI/CD variables are set correctly (`DD_REGISTRY_TOKEN`, `DD_REGISTRY_USERNAME`)
2. Check Vault integration - these credentials are typically auto-configured
3. Ensure you have permission to push to registry.datadoghq.com

### Build Failureswha

- Check that the `TAG` variable is set and follows semantic versioning
- Verify `BUILD_TAGS` includes required Go build tags: `serverless otlp zlib zstd`
- Ensure the datadog-agent source code is in a buildable state
-
## 📚 Related Documentation

- [Releasing a new version](https://datadoghq.atlassian.net/wiki/spaces/SLS/pages/3048800938/Releasing+a+new+version) (Confluence)
- [serverless-init-self-monitoring](https://github.com/DataDog/serverless-init-self-monitoring)
- [public-images pipeline](https://gitlab.ddbuild.io/DataDog/public-images)
