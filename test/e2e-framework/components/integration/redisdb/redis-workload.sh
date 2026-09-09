#!/usr/bin/env bash
# Continuous Redis workload driver. Runs inside the redis 7.2 container and
# exercises the major data-structure commands plus slow operations so that the
# Datadog redisdb check observes meaningful INFO / commandstats / slowlog /
# keyspace / client behaviour. Bounded randomness keeps load realistic but
# light enough for a t3.medium.
set -u

REDIS="redis-cli -h 127.0.0.1 -p 6379"

# Lower the slowlog threshold so DEBUG SLEEP calls are recorded as slow entries.
$REDIS CONFIG SET slowlog-log-slower-than 1000 >/dev/null 2>&1 || true

i=0
while true; do
  i=$(( (i + 1) % 1000 ))
  r=$(( RANDOM % 100 ))

  # String ops: SET/GET/INCR/APPEND
  $REDIS SET "dd:workload:str:${r}" "value-${i}" EX 300 >/dev/null 2>&1
  $REDIS GET "dd:workload:str:${r}" >/dev/null 2>&1
  $REDIS INCR "dd:workload:counter" >/dev/null 2>&1
  $REDIS APPEND "dd:workload:log" "x" >/dev/null 2>&1

  # List ops (bounded length)
  $REDIS LPUSH "dd:workload:list" "item-${i}" >/dev/null 2>&1
  $REDIS LTRIM "dd:workload:list" 0 199 >/dev/null 2>&1

  # Set ops
  $REDIS SADD "dd:workload:set" "member-${r}" >/dev/null 2>&1

  # Hash ops
  $REDIS HSET "dd:workload:hash" "field-${r}" "${i}" >/dev/null 2>&1

  # Sorted-set ops
  $REDIS ZADD "dd:workload:zset" "${r}" "rank-${r}" >/dev/null 2>&1
  $REDIS ZREMRANGEBYRANK "dd:workload:zset" 0 -201 >/dev/null 2>&1

  # Expiry / keyspace churn
  $REDIS EXPIRE "dd:workload:str:${r}" 120 >/dev/null 2>&1

  # Occasionally produce a slow command so SLOWLOG is populated.
  if [ "$r" -lt 5 ]; then
    $REDIS DEBUG SLEEP 0.05 >/dev/null 2>&1 || true
  fi

  # Occasionally open extra short-lived connections for client metrics.
  if [ "$r" -lt 10 ]; then
    ( $REDIS PING >/dev/null 2>&1 & ) 2>/dev/null
  fi

  sleep 0.5
done
