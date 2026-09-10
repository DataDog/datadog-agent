#!/usr/bin/env bash
# Reference script used to produce go-ora-v3-local-test-results.md. Not wired into CI.
#
# Runs the Oracle corecheck suite against a public Oracle image, for one or more git revisions.
# The oracle.test task's own docker lifecycle needs the internal image mirror, so the container
# is started here and the task is run with SKIP_DOCKER=1.
#
# Usage: local-multiversion-test.sh <image> <service_name> <rev> [<rev> ...]
#
# Env overrides:
#   SYS_PASSWORD   sys password to set on the image (default datad0g)
#   PWD_ENV        env var the image reads the password from (default ORACLE_PASSWORD)
#   READY_MARKER   log line marking readiness (default "DATABASE IS READY TO USE")
#   READY_TRIES    readiness polls, 5s apart (default 120)
#   NON_CDB        1 for images without a CDB (11g, 12c XE): seeds a local "datadog" user from
#                  compose/initdb.d instead of the common "c##datadog" user
#   DOCKER_ARGS    extra args for docker run
#
# Examples:
#   local-multiversion-test.sh gvenzl/oracle-xe:21.3.0-slim XE main pr56098
#   NON_CDB=1 local-multiversion-test.sh gvenzl/oracle-xe:11.2.0.2-slim XE main pr56098
#   NON_CDB=1 SYS_PASSWORD=oracle READY_MARKER='Database ready to use' \
#     local-multiversion-test.sh truevoly/oracle-12c xe main pr56098

set -uo pipefail

IMAGE="$1"; SERVICE="$2"; shift 2
CHECK_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO=$(git -C "$CHECK_DIR" rev-parse --show-toplevel)
SYS_PASSWORD="${SYS_PASSWORD:-datad0g}"
PWD_ENV="${PWD_ENV:-ORACLE_PASSWORD}"
READY_MARKER="${READY_MARKER:-DATABASE IS READY TO USE}"
READY_TRIES="${READY_TRIES:-120}"
TAG=$(echo "$IMAGE" | tr '/:.' '___')
CONTAINER="oracle-test-$TAG"

if docker ps --format '{{.Ports}}' | grep -q '0.0.0.0:1521'; then
  echo "!!! port 1521 already used by: $(docker ps --filter publish=1521 --format '{{.Names}}')"
  exit 1
fi

echo ">>> starting $IMAGE"
# shellcheck disable=SC2086
docker run -d --name "$CONTAINER" -p 1521:1521 -e "$PWD_ENV=$SYS_PASSWORD" ${DOCKER_ARGS:-} "$IMAGE" >/dev/null || exit 1

echo -n ">>> waiting for database"
for _ in $(seq 1 "$READY_TRIES"); do
  if docker logs "$CONTAINER" 2>&1 | grep -q "$READY_MARKER"; then ready=1; break; fi
  echo -n "."; sleep 5
done
echo
if [ "${ready:-0}" != 1 ]; then
  echo "!!! database never became ready"
  docker logs --tail 30 "$CONTAINER"
  docker rm -f "$CONTAINER" >/dev/null
  exit 1
fi

test_user_env=()
if [ "${NON_CDB:-0}" = 1 ]; then
  # TestMain runs compose/initdb.d as sys, but those scripts create the common user c##datadog,
  # which only exists in a CDB. Seed the equivalent local user up front.
  seed=/tmp/initdb-noncdb.sql
  {
    echo "CREATE USER datadog IDENTIFIED BY datadog;"
    echo "GRANT CREATE SESSION TO datadog;"
    for f in "$CHECK_DIR"/compose/initdb.d/*.sql; do
      case "$f" in *00-create-user.sql) continue;; esac
      sed -e 's/c##datadog/datadog/g' -e 's/CONTAINER=ALL//g' "$f"
      echo
      case "$f" in *.nosplit.sql) echo "/";; esac
    done
    echo "EXIT;"
  } > "$seed"
  docker cp "$seed" "$CONTAINER":/tmp/initdb-noncdb.sql >/dev/null
  echo ">>> seeding local datadog user"
  docker exec "$CONTAINER" bash -lc \
    "sqlplus -S sys/$SYS_PASSWORD@localhost:1521/\$ORACLE_SID as sysdba @/tmp/initdb-noncdb.sql" \
    | grep -E 'ORA-|created' | sort | uniq -c
  test_user_env=(ORACLE_TEST_USER=datadog ORACLE_TEST_PASSWORD=datadog)
fi

for REV in "$@"; do
  echo ">>> checking out $REV"
  git -C "$REPO" checkout -q "$REV" || exit 1
  LOG="/tmp/oracle-${TAG}-${REV}.log"
  ( cd "$REPO" && env SKIP_DOCKER=1 ORACLE_TEST_SERVICE_NAME="$SERVICE" \
      ORACLE_TEST_SYS_PASSWORD="$SYS_PASSWORD" "${test_user_env[@]}" \
      timeout 3600 dda inv -- oracle.test -v ) > "$LOG" 2>&1
  echo ">>> $IMAGE / $REV: $(grep -ac -- '--- PASS' "$LOG") passed, $(grep -ac -- '--- FAIL' "$LOG") failed, $(grep -ac -- '--- SKIP' "$LOG") skipped  (log: $LOG)"
  grep -a -- '--- FAIL' "$LOG" | sed 's/^/      /'
done

docker rm -f "$CONTAINER" >/dev/null
