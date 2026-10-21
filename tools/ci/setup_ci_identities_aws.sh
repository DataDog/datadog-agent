#!/bin/bash
# Configures AWS credentials for the job via CI Identities.
#
# The 6.53.x Linux build images authenticated to AWS through the runner's instance
# profile, which no longer grants access to dd-ci-artefacts-build-stable (S3 403).
# Follow what main does: assume a per-job role with ci-identities-gitlab-job-client
# and have the AWS CLI read those credentials from AWS_SHARED_CREDENTIALS_FILE.
#
# ci-identities-gitlab-job-client is not baked into the 6.53.x deb_x64 image, so it
# is runtime-installed here (accepted agent6 workaround). The client binary is
# fetched from binaries-ddbuild-io-prod, a bucket the instance profile can still
# read (only the artefacts bucket lost access), so there is no bootstrap problem.
#
# Requires the CI_IDENTITIES_GITLAB_ID_TOKEN id_token and AWS_SHARED_CREDENTIALS_FILE
# to be set on the job.
set -euo pipefail

: "${AWS_SHARED_CREDENTIALS_FILE:?AWS_SHARED_CREDENTIALS_FILE must be set in the job variables}"

CI_IDENTITIES_VERSION="${CI_IDENTITIES_VERSION:-v0.6.7}"
CI_IDENTITIES_SHA256="${CI_IDENTITIES_SHA256:-c39c35f1db9daddaca6b47080e5bcb4b8ffd5cf7e7fe61b02ed56690e749928c}"

script_dir="$(dirname "$0")"

if ! command -v ci-identities-gitlab-job-client >/dev/null 2>&1; then
    case "$(uname -m)" in
        x86_64) arch="amd64" ;;
        aarch64) arch="arm64" ;;
        *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
    esac
    bin="ci-identities-gitlab-job-client-linux-${arch}"
    src="s3://binaries-ddbuild-io-prod/ci-identities/ci-identities-gitlab-job-client/versions/${CI_IDENTITIES_VERSION}/${bin}"
    "${script_dir}/retry.sh" aws s3 cp --only-show-errors "$src" "/usr/local/bin/ci-identities-gitlab-job-client"
    if [ "$arch" = "amd64" ]; then
        echo "${CI_IDENTITIES_SHA256}  /usr/local/bin/ci-identities-gitlab-job-client" | sha256sum --check
    fi
    chmod +x /usr/local/bin/ci-identities-gitlab-job-client
fi

mkdir -p "$(dirname "$AWS_SHARED_CREDENTIALS_FILE")"
ci-identities-gitlab-job-client assume-role
