
ARTIFACTS_DIR="/artifacts/${CI_JOB_ID}"
mkdir -p "${ARTIFACTS_DIR}"

export DD_API_KEY=deadbeeffacefeeddeadbeeffacefeed
export DD_LOG_LEVEL=error
export DD_APM_MAX_MEMORY=5GB
export DD_APM_MAX_CPU_PERCENT=200
export DD_APM_REMOTE_TAGGER=false

go run ./cmd/trace-agent
k6 run ./test/benchmarks/apm_scripts/k6_basic.js
killall -9 trace-agent

git checkout main
go run ./cmd/trace-agent
k6 run ./test/benchmarks/apm_scripts/k6_basic.js
killall -9 trace-agent

git checkout "${CI_COMMIT_REF_NAME}" # (Only needed while these changes aren't merged to main)