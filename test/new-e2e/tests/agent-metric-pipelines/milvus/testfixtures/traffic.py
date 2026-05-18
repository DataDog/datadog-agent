"""Continuously exercise a Milvus instance to produce realistic metrics.

This is the workload used by the E2E scenario in
``test/new-e2e/tests/agent-metric-pipelines/milvus``. It connects to a
Milvus standalone deployment, creates (or reuses) a small collection,
and runs an unbounded insert/search/query loop. The goal is not
correctness of the data but to keep proxy / query node / data node
metrics non-zero so the Datadog integration has something to report.
"""

import os
import random
import time

from pymilvus import (
    Collection,
    CollectionSchema,
    DataType,
    FieldSchema,
    connections,
    utility,
)

HOST = os.environ.get("MILVUS_HOST", "milvus")
PORT = os.environ.get("MILVUS_PORT", "19530")
COLLECTION = "e2e_traffic"
DIM = 16


def get_or_create_collection() -> Collection:
    if utility.has_collection(COLLECTION):
        return Collection(COLLECTION)

    fields = [
        FieldSchema(name="id", dtype=DataType.INT64, is_primary=True, auto_id=True),
        FieldSchema(name="embedding", dtype=DataType.FLOAT_VECTOR, dim=DIM),
    ]
    schema = CollectionSchema(fields, description="E2E traffic collection")
    collection = Collection(COLLECTION, schema)
    collection.create_index(
        field_name="embedding",
        index_params={
            "index_type": "IVF_FLAT",
            "metric_type": "L2",
            "params": {"nlist": 16},
        },
    )
    return collection


def random_vectors(n: int) -> list[list[float]]:
    return [[random.random() for _ in range(DIM)] for _ in range(n)]


def main() -> None:
    connections.connect(alias="default", host=HOST, port=PORT)
    collection = get_or_create_collection()
    collection.load()

    iteration = 0
    while True:
        iteration += 1
        try:
            # Insert a small batch so the writer path is exercised.
            collection.insert([random_vectors(32)])

            # Flush occasionally to drive data node metrics.
            if iteration % 10 == 0:
                collection.flush()

            # Search to drive query node + proxy metrics.
            collection.search(
                data=random_vectors(4),
                anns_field="embedding",
                param={"metric_type": "L2", "params": {"nprobe": 8}},
                limit=5,
            )

            # Lightweight query to add variety.
            if iteration % 5 == 0:
                collection.query(expr="id >= 0", limit=1, output_fields=["id"])

            print(f"iteration={iteration} ok", flush=True)
        except Exception as exc:  # noqa: BLE001 - keep the loop alive
            print(f"iteration={iteration} error={exc!r}", flush=True)
            time.sleep(5)
            # Re-connect on hard errors.
            try:
                connections.disconnect("default")
            except Exception:  # noqa: BLE001
                pass
            connections.connect(alias="default", host=HOST, port=PORT)
            collection = get_or_create_collection()
            collection.load()

        time.sleep(1)


if __name__ == "__main__":
    main()
