#!/usr/bin/env bash
set -euo pipefail

# Build and push the observer-eval gs-flow generator image.
#
# This script copies episode data from the gensim-episodes repo into the
# Docker build context so episodes are baked into the image.
#
# Usage:
#   ./build.sh --push                                    # all episodes
#   ./build.sh --push --episodes "059_Fortnite:memcached" # only specified episodes
#   GENSIM_REPO_PATH=/path/to/repo ./build.sh --push

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REGISTRY="${REGISTRY:-us-east1-docker.pkg.dev/dd-plt-simulation-environment/gensim-images}"
IMAGE_NAME="observer-eval"
TAG="${TAG:-latest}"
FULL_IMAGE="$REGISTRY/$IMAGE_NAME:$TAG"

PUSH=false
EPISODE_FILTER=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --push) PUSH=true; shift;;
    --episodes) EPISODE_FILTER="$2"; shift 2;;
    *) echo "Unknown arg: $1"; exit 1;;
  esac
done

if [ -z "${GENSIM_REPO_PATH:-}" ] || [ ! -d "${GENSIM_REPO_PATH:-}" ]; then
  echo "ERROR: Set GENSIM_REPO_PATH to your gensim-episodes checkout."
  echo "  export GENSIM_REPO_PATH=/path/to/gensim-episodes"
  exit 1
fi

echo "Using gensim-episodes: $GENSIM_REPO_PATH"

# Parse episode names from filter (episode:scenario pairs → just episode names)
WANTED_EPISODES=()
if [ -n "$EPISODE_FILTER" ]; then
  IFS=',' read -ra PAIRS <<< "$EPISODE_FILTER"
  for pair in "${PAIRS[@]}"; do
    WANTED_EPISODES+=("${pair%%:*}")
  done
  echo "Filtering to episodes: ${WANTED_EPISODES[*]}"
fi

# Copy episodes into build context
EPISODES_DIR="$SCRIPT_DIR/episodes"
rm -rf "$EPISODES_DIR"
mkdir -p "$EPISODES_DIR"

# Copy _shared helpers
if [ -d "$GENSIM_REPO_PATH/_shared" ]; then
  cp -r "$GENSIM_REPO_PATH/_shared" "$EPISODES_DIR/_shared"
elif [ -d "$GENSIM_REPO_PATH/postmortems/_shared" ]; then
  cp -r "$GENSIM_REPO_PATH/postmortems/_shared" "$EPISODES_DIR/_shared"
fi

# Find and copy matching episodes
for subdir in agent-q-branch postmortems synthetics; do
  if [ ! -d "$GENSIM_REPO_PATH/$subdir" ]; then
    continue
  fi
  for ep_dir in "$GENSIM_REPO_PATH/$subdir"/*/; do
    ep_name="$(basename "$ep_dir")"
    [ "$ep_name" = "_shared" ] && continue

    # Skip if filter is set and this episode isn't in it
    if [ ${#WANTED_EPISODES[@]} -gt 0 ]; then
      found=false
      for wanted in "${WANTED_EPISODES[@]}"; do
        [ "$wanted" = "$ep_name" ] && found=true && break
      done
      [ "$found" = false ] && continue
    fi

    if [ -d "$ep_dir/chart" ] && [ -f "$ep_dir/play-episode.sh" ]; then
      dest="$EPISODES_DIR/$ep_name"
      mkdir -p "$dest"
      tar czf "$dest/chart.tar.gz" -C "$ep_dir" chart/
      cp "$ep_dir/play-episode.sh" "$dest/"
      [ -d "$ep_dir/episodes" ] && cp -r "$ep_dir/episodes" "$dest/episodes"
      [ -f "$ep_dir/docker-compose.yaml" ] && cp "$ep_dir/docker-compose.yaml" "$dest/"
      [ -d "$ep_dir/services" ] && cp -r "$ep_dir/services" "$dest/services"
    fi
  done
done

EP_COUNT=$(find "$EPISODES_DIR" -maxdepth 1 -mindepth 1 -type d ! -name '_shared' | wc -l)
echo "Baked $EP_COUNT episodes into build context"
du -sh "$EPISODES_DIR" | awk '{print "Total size: " $1}'

# Build and push episode service images (from docker-compose.yaml)
if [ "$PUSH" = true ]; then
  for ep_dir in "$EPISODES_DIR"/*/; do
    ep_name="$(basename "$ep_dir")"
    [ "$ep_name" = "_shared" ] && continue
    compose_file="$ep_dir/docker-compose.yaml"
    [ ! -f "$compose_file" ] && continue

    echo "Building service images for episode: $ep_name"
    IMAGES=$(grep '  image:' "$compose_file" | awk '{print $2}')

    # Build each service individually for linux/amd64 (docker compose build
    # doesn't reliably support --platform across all versions)
    if [ -d "$ep_dir/services" ]; then
      for svc_dir in "$ep_dir/services"/*/; do
        [ ! -f "$svc_dir/Dockerfile" ] && continue
        svc_name="$(basename "$svc_dir")"
        # Find the image name from docker-compose.yaml for this service
        svc_image=$(awk "/$svc_name:/{found=1} found && /image:/{print \$2; exit}" "$compose_file")
        if [ -n "$svc_image" ]; then
          echo "  Building $svc_image from $svc_dir (linux/amd64)"
          docker buildx build --platform linux/amd64 -t "$svc_image" "$svc_dir" --load
        fi
      done
    fi

    # Tag and push to GAR
    for IMG in $IMAGES; do
      GAR_IMG="$REGISTRY/$IMG"
      echo "  Pushing $GAR_IMG"
      docker tag "$IMG" "$GAR_IMG" 2>/dev/null || true
      docker push "$GAR_IMG" || echo "WARN: failed to push $GAR_IMG"
    done
  done
fi

if [ "$PUSH" = true ]; then
  echo "Building and pushing $FULL_IMAGE (linux/amd64)..."
  docker buildx build --platform linux/amd64 -t "$FULL_IMAGE" "$SCRIPT_DIR" --push
else
  echo "Building $FULL_IMAGE (linux/amd64)..."
  docker buildx build --platform linux/amd64 -t "$FULL_IMAGE" "$SCRIPT_DIR" --load
fi

# Clean up copied episodes
rm -rf "$EPISODES_DIR"
