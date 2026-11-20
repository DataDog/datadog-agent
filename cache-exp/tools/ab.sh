#!/usr/bin/env bash
set -uo pipefail
WT=$(git rev-parse --show-toplevel)
BAZEL=${BAZEL:-bazelisk}
OUT=$WT/cache-exp/data/ab
mkdir -p "$OUT"
cd "$WT"

cpu_ticks() {
  awk '{print $14 + $15}' "/proc/$1/stat"
}

run() {
  name=$1
  shift
  echo "=== $name $(date +%T)"
  $BAZEL clean >/dev/null 2>&1
  pid=$($BAZEL info server_pid 2>/dev/null)
  before=$(cpu_ticks "$pid")
  start=$(date +%s.%N)
  $BAZEL build --disk_cache= --remote_download_all "$@" //cmd/agent >"$OUT/$name.build.log" 2>&1
  rc=$?
  end=$(date +%s.%N)
  after=$(cpu_ticks "$pid")
  hz=$(getconf CLK_TCK)
  printf 'rc=%d wall=%.1fs server_cpu=%.1fs\n' "$rc" "$(echo "$end - $start" | bc)" "$(echo "($after - $before) / $hz" | bc -l)" | tee "$OUT/$name.stats"
  grep -E "processes:|Elapsed time" "$OUT/$name.build.log" | tail -2 | tee -a "$OUT/$name.stats"
  for f in bazel-invocation.*; do cp "$f" "$OUT/$name.$f"; done
  [ $rc -eq 0 ] || exit 1
}

run warmup
for i in 1 2 3; do
  run "off$i"
  run "on$i" --remote_cache_compression
done
