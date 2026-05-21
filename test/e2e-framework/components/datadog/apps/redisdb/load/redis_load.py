#!/usr/bin/env python3
# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2025-present Datadog, Inc.
#
# Continuous Redis load generator for the redisdb E2E lab.
#
# Metric coverage this script targets:
#   redis.keys / redis.expires / redis.persist                (SET/SETEX/PERSIST)
#   redis.stats.keyspace_hits / redis.stats.keyspace_misses   (GET hit + miss)
#   redis.key.length (list/set/zset/hash/stream)              (LPUSH/SADD/ZADD/HSET/XADD against test_*)
#   redis.net.commands (rate)                                  (all commands)
#   redis.command.calls / redis.command.usec_per_call          (command_stats: true on agent side)
#   redis.clients.blocked                                      (BLPOP held open in background thread)
#   redis.pubsub.channels                                      (SUBSCRIBE + PUBLISH in background)
#   redis.slowlog.micros.*                                     (CONFIG SET slowlog=0 + SORT)
#   redis.rdb.bgsave / redis.perf.latest_fork_usec             (BGSAVE every 60 s)
#   redis.mem.lua / redis.scripts.cached                       (EVAL every 60 s)
#   redis.replication.delay                                    (writes on master replicate to replica)

import os
import random
import string
import threading
import time

import redis

# ── connection parameters (injected via environment) ──────────────────────────
MASTER_HOST = os.environ.get("REDIS_MASTER_HOST", "redis-master")
MASTER_PORT = int(os.environ.get("REDIS_MASTER_PORT", "6382"))
REPLICA_HOST = os.environ.get("REDIS_REPLICA_HOST", "redis-replica")
REPLICA_PORT = int(os.environ.get("REDIS_REPLICA_PORT", "6380"))

# db 14 – same db used by the integrations-core test suite for seeded data
DB_INDEX = 14

# ── helpers ───────────────────────────────────────────────────────────────────


def rand_str(n: int = 8) -> str:
    return "".join(random.choices(string.ascii_lowercase + string.digits, k=n))


def rand_score() -> float:
    return round(random.uniform(0, 1_000_000), 4)


def wait_for_redis(host: str, port: int, db: int = 0, password: str | None = None, timeout: int = 120) -> redis.Redis:
    """Block until Redis is reachable, then return a connected client."""
    deadline = time.time() + timeout
    while True:
        try:
            r = redis.Redis(host=host, port=port, db=db, password=password, socket_connect_timeout=2)
            r.ping()
            print(f"Connected to Redis at {host}:{port}", flush=True)
            return r
        except Exception as exc:
            if time.time() > deadline:
                raise RuntimeError(f"Redis at {host}:{port} not ready after {timeout}s: {exc}") from exc
            print(f"Waiting for Redis at {host}:{port}: {exc}", flush=True)
            time.sleep(2)


def wait_for_replica(master: redis.Redis, timeout: int = 60) -> None:
    """Wait until master reports at least one connected replica."""
    deadline = time.time() + timeout
    while True:
        info = master.info("replication")
        if info.get("connected_slaves", 0) >= 1:
            print("Replica connected to master.", flush=True)
            return
        if time.time() > deadline:
            print("WARNING: No replica connected after waiting; continuing anyway.", flush=True)
            return
        time.sleep(2)


# ── seed initial data ─────────────────────────────────────────────────────────


def seed_data(master: redis.Redis) -> None:
    """Write a deterministic seed so key-length metrics are non-empty from tick 0."""
    pipe = master.pipeline(transaction=False)

    # list – 3 entries
    for i in range(1, 4):
        pipe.lpush("test_key1", f"seed-list-{i}")

    # stream – 2 entries (test_key4 matches the integrations-core test pattern)
    pipe.xadd("test_key4", {"field": "seed-0"})
    pipe.xadd("test_key4", {"field": "seed-1"})

    # plain string keys
    pipe.set("key1", "seedval1")
    pipe.set("key2", "seedval2")

    # key with TTL → contributes to redis.expires
    pipe.setex("expirekey", 1000, "seedval3")

    # hash, zset, set – initial members
    pipe.hset("test_hash", mapping={"seed_field": "seed_val"})
    pipe.zadd("test_zset", {"seed_member": rand_score()})
    pipe.sadd("test_set", "seed_member_0")

    # write a key on master so replica can confirm replication
    pipe.set("replicated:test", "true")

    pipe.execute()
    print("Seed data written.", flush=True)


# ── background threads ────────────────────────────────────────────────────────


def _blpop_loop(host: str, port: int) -> None:
    """
    Hold a BLPOP on a key that will never receive a push.
    This keeps redis.clients.blocked > 0 as seen by the check.
    Use a dedicated connection so it never competes with the write loop.
    """
    while True:
        try:
            r = redis.Redis(host=host, port=port, db=DB_INDEX, socket_connect_timeout=5)
            # timeout=0 means block indefinitely; the server will drop the
            # connection after server-side idle timeout (default 0 = no timeout),
            # so we retry in the outer loop if it ever returns.
            r.blpop(["blocked_key_lab"], timeout=0)
        except Exception as exc:
            print(f"BLPOP thread error (will retry): {exc}", flush=True)
            time.sleep(3)


def _pubsub_subscriber_loop(host: str, port: int) -> None:
    """
    Keep a persistent SUBSCRIBE open so redis.pubsub.channels > 0.
    """
    while True:
        try:
            r = redis.Redis(host=host, port=port, db=DB_INDEX, socket_connect_timeout=5)
            ps = r.pubsub()
            ps.subscribe("channel:lab")
            print("PubSub subscriber active on channel:lab", flush=True)
            # drain messages; get_message is non-blocking, so sleep between calls
            while True:
                ps.get_message(timeout=1.0)
                time.sleep(0.1)
        except Exception as exc:
            print(f"PubSub subscriber error (will retry): {exc}", flush=True)
            time.sleep(3)


# ── main load loop ────────────────────────────────────────────────────────────


def run_load(master: redis.Redis) -> None:
    tick = 0
    last_slow = 0
    last_heavy = 0

    print("Load loop started.", flush=True)

    while True:
        now = time.time()

        # ── every tick (~1 s): core throughput ──────────────────────────────
        try:
            pipe = master.pipeline(transaction=False)

            # random SET → contributes to redis.keys, redis.mem.used, redis.net.commands
            rkey = f"key:{rand_str(6)}"
            pipe.set(rkey, rand_str(32))

            # SETEX → contributes to redis.expires
            ekey = f"expkey:{rand_str(6)}"
            pipe.setex(ekey, 30, rand_str(16))

            pipe.execute()

            # GETs: alternate existing vs missing to produce hits and misses
            if tick % 2 == 0:
                master.get(rkey)  # guaranteed hit (just set above)
            else:
                master.get(f"missing:{rand_str(8)}")  # guaranteed miss

        except Exception as exc:
            print(f"Tick {tick} error: {exc}", flush=True)

        # ── every 3 ticks (~3 s): key-length data types ─────────────────────
        if tick % 3 == 0:
            try:
                pipe = master.pipeline(transaction=False)
                pipe.lpush("test_key1", rand_str(12))  # list  → redis.key.length (list)
                pipe.xadd("test_key4", {"f": rand_str(8)})  # stream → redis.key.length (stream)
                pipe.hset("test_hash", f"f{tick}", rand_str(8))  # hash   → redis.key.length (hash)
                pipe.zadd("test_zset", {f"m{tick}": rand_score()})  # zset   → redis.key.length (zset)
                pipe.sadd("test_set", rand_str(6))  # set    → redis.key.length (set)
                pipe.execute()
            except Exception as exc:
                print(f"Data-types tick error: {exc}", flush=True)

        # ── every 5 ticks (~5 s): PUBLISH keeps pubsub.channels counter alive
        if tick % 5 == 0:
            try:
                master.publish("channel:lab", f"payload:{rand_str(16)}")
            except Exception as exc:
                print(f"PUBLISH error: {exc}", flush=True)

        # ── every 30 ticks (~30 s): trigger slowlog entries ──────────────────
        if now - last_slow >= 30:
            last_slow = now
            try:
                # Lower the threshold so the next commands always appear in the slowlog.
                master.config_set("slowlog-log-slower-than", 0)
                # SORT on a list with several members is expensive enough.
                master.sort("test_key1", alpha=True)  # redis.slowlog.micros.*
                # KEYS * also shows up but is too dangerous in large DBs; SORT is safer.
                master.config_set("slowlog-log-slower-than", 10000)
                print(f"Slowlog trigger done at tick {tick}", flush=True)
            except Exception as exc:
                print(f"Slowlog trigger error: {exc}", flush=True)
                # Restore threshold even if SORT failed
                try:
                    master.config_set("slowlog-log-slower-than", 10000)
                except Exception:
                    pass

        # ── every 60 ticks (~60 s): BGSAVE + EVAL ───────────────────────────
        if now - last_heavy >= 60:
            last_heavy = now
            try:
                # BGSAVE → redis.rdb.bgsave (transitional > 0), redis.perf.latest_fork_usec
                master.bgsave()
                print(f"BGSAVE triggered at tick {tick}", flush=True)
            except redis.exceptions.ResponseError as exc:
                # "Background save already in progress" is harmless
                print(f"BGSAVE skipped: {exc}", flush=True)
            except Exception as exc:
                print(f"BGSAVE error: {exc}", flush=True)

            try:
                # EVAL → redis.mem.lua, redis.scripts.cached
                master.eval("return redis.call('dbsize')", 0)
                print(f"EVAL triggered at tick {tick}", flush=True)
            except Exception as exc:
                print(f"EVAL error: {exc}", flush=True)

        tick += 1
        time.sleep(1)


# ── entry point ───────────────────────────────────────────────────────────────


def main() -> None:
    master = wait_for_redis(MASTER_HOST, MASTER_PORT, db=DB_INDEX)
    # Wait for replica so replication metrics are immediately populated
    wait_for_replica(master)

    seed_data(master)

    # Background: keep one connection blocked on BLPOP
    t_blpop = threading.Thread(
        target=_blpop_loop,
        args=(MASTER_HOST, MASTER_PORT),
        daemon=True,
        name="blpop-thread",
    )
    t_blpop.start()

    # Background: keep a pub/sub subscriber alive
    t_sub = threading.Thread(
        target=_pubsub_subscriber_loop,
        args=(MASTER_HOST, MASTER_PORT),
        daemon=True,
        name="pubsub-thread",
    )
    t_sub.start()

    run_load(master)


if __name__ == "__main__":
    main()
