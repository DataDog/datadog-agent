#!/usr/bin/env python3
"""Scrappy model scorer — stateful per-tick inference over stdin/stdout.

Uses model.score_tick() for per-tick inference with carried SSM/SWA state.
All state management and salience extraction is internal to the model.

Protocol (one JSON object per line):
    Request:  {"token_ids": [4, 100, 200, 5]}
    Response: {"score": 0.123, "tokens_processed": 4, "elapsed_ms": 7.9}

    Alert with salience:
    Response: {"score": 0.85, ..., "salience": [{"token_id": 42, "weight": 0.12}, ...]}

    Reset state: {"reset": true}
    Response:    {"reset": true}

    Handshake (first line from scorer):
    {"ready": true, "vocab_size": 3594, "params": 108443904, "mode": "score_tick"}

    Shutdown: EOF on stdin → clean exit.
"""

import argparse
import json
import sys
import time
from pathlib import Path

import torch


def load_model(checkpoint_path: str):
    """Load a ScrappyModel from a .pt checkpoint."""
    scrappy_src = Path.home() / "dd" / "scrappy" / "src"
    if str(scrappy_src) not in sys.path:
        sys.path.insert(0, str(scrappy_src))

    from scrappy.model import ScrappyConfig, ScrappyModel

    checkpoint = torch.load(checkpoint_path, map_location="cpu", weights_only=False)

    if "config" in checkpoint:
        config_dict = checkpoint["config"]
        if isinstance(config_dict, ScrappyConfig):
            config = config_dict
        else:
            config = ScrappyConfig(**config_dict)
    else:
        config = ScrappyConfig()

    model = ScrappyModel(config)
    model.load_state_dict(checkpoint["model_state_dict"])
    model.eval()

    return model, config


def main():
    parser = argparse.ArgumentParser(description="Scrappy model scorer")
    parser.add_argument("checkpoint", help="Path to .pt checkpoint")
    parser.add_argument(
        "--salience-topk", type=int, default=10, help="Top-K salient tokens to return on alert (0 to disable)"
    )
    parser.add_argument(
        "--threshold",
        type=float,
        default=0.5,
        help="Unified threshold for alert detection, autoregressive feedback, and salience extraction",
    )
    args = parser.parse_args()

    t0 = time.monotonic()
    print(f"Loading model from {args.checkpoint}...", file=sys.stderr)
    model, config = load_model(args.checkpoint)
    load_time = time.monotonic() - t0
    # Unify feedback threshold with detection threshold
    model._feedback_threshold = args.threshold
    print(
        f"Model loaded in {load_time:.1f}s: {model.param_count():,} params, "
        f"vocab={config.vocab_size}, threshold={args.threshold} (unified)",
        file=sys.stderr,
    )

    # Handshake
    handshake = {
        "ready": True,
        "vocab_size": config.vocab_size,
        "params": model.param_count(),
        "mode": "score_tick",
        "salience_topk": args.salience_topk,
        "threshold": args.threshold,
    }
    sys.stdout.write(json.dumps(handshake) + "\n")
    sys.stdout.flush()

    # Serve scoring requests
    tick_count = 0
    total_score_ms = 0.0

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            req = json.loads(line)

            if req.get("reset"):
                model.clear_states()
                resp = {"reset": True}
            else:
                token_ids = req["token_ids"]

                t0 = time.monotonic()
                p_alert, salience = model.score_tick(
                    token_ids,
                    threshold=args.threshold,
                    salience_topk=args.salience_topk,
                )
                elapsed_ms = (time.monotonic() - t0) * 1000

                tick_count += 1
                total_score_ms += elapsed_ms

                resp = {
                    "score": p_alert,
                    "tokens_processed": len(token_ids),
                    "elapsed_ms": round(elapsed_ms, 1),
                }
                if salience is not None:
                    resp["salience"] = [
                        {"token_id": tid, "weight": w}
                        for tid, w in zip(salience.token_ids, salience.weights, strict=False)
                    ]

                if tick_count % 100 == 0:
                    avg_ms = total_score_ms / tick_count
                    print(
                        f"Scored {tick_count} ticks, last={elapsed_ms:.1f}ms, "
                        f"avg={avg_ms:.1f}ms, tokens={len(token_ids)}",
                        file=sys.stderr,
                    )
        except Exception as e:
            resp = {"error": str(e), "score": 0.0}

        sys.stdout.write(json.dumps(resp) + "\n")
        sys.stdout.flush()

    if tick_count > 0:
        avg_ms = total_score_ms / tick_count
        print(
            f"Scorer shutting down: {tick_count} ticks, avg={avg_ms:.1f}ms",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
