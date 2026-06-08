#!/usr/bin/env python3
"""Bench: step_chunk inference on JSONL replay data.

Compares per-token step() vs batched step_chunk() on real episode data.
Also tests worst-case tick sizes from the training corpus.

Usage:
    ~/dd/scrappy/.venv-eval/bin/python3 scrappy_bench_test.py \
        --jsonl ~/data/scrappy/replay/causalgen-2026-04/release-b7376249-1776331941710416000/scrappy-collect.jsonl \
        --vocab ~/dd/scrappy/vocab.json \
        --checkpoint ~/dd/scrappy/checkpoints/v0.1-run-001/epoch_012.pt \
        --ticks 60
"""

import argparse
import sys
import time
from pathlib import Path

scrappy_src = Path.home() / "dd" / "scrappy" / "src"
if str(scrappy_src) not in sys.path:
    sys.path.insert(0, str(scrappy_src))

import torch  # noqa: E402
import torch.nn.functional as F  # noqa: E402
from scrappy.data.jsonl_reader import read_jsonl  # noqa: E402
from scrappy.model import ScrappyConfig, ScrappyModel  # noqa: E402
from scrappy.tokenizer import Tokenizer  # noqa: E402
from scrappy.vocabulary import Vocabulary  # noqa: E402


def load_model(checkpoint_path: str):
    checkpoint = torch.load(checkpoint_path, map_location="cpu", weights_only=False)
    if "config" in checkpoint:
        cd = checkpoint["config"]
        config = cd if isinstance(cd, ScrappyConfig) else ScrappyConfig(**cd)
    else:
        config = ScrappyConfig()
    model = ScrappyModel(config)
    model.load_state_dict(checkpoint["model_state_dict"])
    model.eval()
    return model, config


def p_alert_from_logits(logits: torch.Tensor, pos: int = -1) -> float:
    """Extract P([alert]) from logits at given position."""
    tick_logits = logits[0, pos]
    probs = F.softmax(tick_logits[[10, 11]], dim=-1)
    return probs[1].item()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--jsonl", required=True)
    parser.add_argument("--vocab", required=True)
    parser.add_argument("--checkpoint", required=True)
    parser.add_argument("--ticks", type=int, default=60)
    parser.add_argument("--quantize", action="store_true")
    args = parser.parse_args()

    vocab = Vocabulary.load(args.vocab)
    tokenizer = Tokenizer(vocab=vocab, zero_suppress=True, shuffle_slots=False)
    print(f"Vocab: {vocab.size} tokens")

    t0 = time.monotonic()
    model, config = load_model(args.checkpoint)
    # Init state BEFORE quantization (quantized modules break .weight.dtype access)
    init_states = model.init_state(batch_size=1)

    if args.quantize:
        torch.backends.quantized.engine = "qnnpack"
        model = torch.ao.quantization.quantize_dynamic(
            model,
            {torch.nn.Linear},
            dtype=torch.qint8,
        )
        print("INT8 dynamic quantization applied (qnnpack)")
    print(f"Model: {model.param_count():,} params, loaded in {(time.monotonic()-t0)*1000:.0f}ms")

    # --- Read ticks ---
    ticks = []
    for tick in read_jsonl(args.jsonl):
        if tick.series:
            ticks.append(tick)
        if len(ticks) >= args.ticks:
            break
    tokenizer.scan_structural_signatures(ticks)
    print(f"Read {len(ticks)} ticks")

    # --- Tokenize all ticks first ---
    tick_data = []
    for tick in ticks:
        tokens = tokenizer.tokenize_tick(tick, phase=None)
        tick_data.append((tick.data_time, len(tick.series), tokens))

    # Find distribution stats
    all_lens = [len(t[2]) for t in tick_data]
    all_lens.sort()
    print(
        f"Token lengths: min={min(all_lens)}, median={all_lens[len(all_lens)//2]}, "
        f"max={max(all_lens)}, mean={sum(all_lens)/len(all_lens):.0f}"
    )
    print()

    # ==========================================================
    # Bench: step_chunk
    # ==========================================================
    print("=" * 110)
    print("step_chunk (batched per-tick inference)")
    print("=" * 110)

    import copy

    states = copy.deepcopy(init_states)
    pos = 0
    total_tokens = 0

    hdr = f"{'Tick':>4}  {'DataTime':>12}  {'Series':>6}  {'Tokens':>6}  {'TotalTok':>8}  {'ms':>8}  {'ms/tok':>7}  {'P(alert)':>9}  {'Pred':>7}"
    print(hdr)
    print("-" * len(hdr))

    chunk_times = []
    for i, (data_time, n_series, tokens) in enumerate(tick_data):
        token_tensor = torch.tensor([tokens], dtype=torch.long)

        t0 = time.monotonic()
        with torch.no_grad():
            logits, states = model.step_chunk(token_tensor, states, pos)
        elapsed_ms = (time.monotonic() - t0) * 1000

        pos += len(tokens)
        total_tokens += len(tokens)
        chunk_times.append(elapsed_ms)

        p_alert = p_alert_from_logits(logits)
        prediction = "ALERT" if p_alert >= 0.5 else "normal"
        ms_per_tok = elapsed_ms / len(tokens) if tokens else 0

        print(
            f"{i+1:>4}  {data_time:>12}  {n_series:>6}  "
            f"{len(tokens):>6}  {total_tokens:>8}  {elapsed_ms:>8.1f}  "
            f"{ms_per_tok:>7.2f}  {p_alert:>9.4f}  {prediction:>7}"
        )

    print()
    print(
        f"step_chunk summary: {len(chunk_times)} ticks, "
        f"avg={sum(chunk_times)/len(chunk_times):.1f}ms, "
        f"min={min(chunk_times):.1f}ms, max={max(chunk_times):.1f}ms"
    )

    # ==========================================================
    # Synthetic worst-case: large tick sizes
    # ==========================================================
    print()
    print("=" * 70)
    print("Synthetic worst-case ticks (random tokens, step_chunk)")
    print("=" * 70)

    for n_tokens in [100, 250, 500, 1000, 2000, 4000, 8000, 15000]:
        # Fresh state for each test
        test_states = copy.deepcopy(init_states)
        fake_tokens = torch.randint(0, config.vocab_size, (1, n_tokens), dtype=torch.long)

        t0 = time.monotonic()
        with torch.no_grad():
            logits, _ = model.step_chunk(fake_tokens, test_states, 0)
        elapsed_ms = (time.monotonic() - t0) * 1000
        ms_per_tok = elapsed_ms / n_tokens

        within = "OK" if elapsed_ms < 1000 else "OVER"
        print(f"  {n_tokens:>6} tokens: {elapsed_ms:>8.1f}ms  ({ms_per_tok:.2f} ms/tok)  [{within}]")

    print()
    print("Done.")


if __name__ == "__main__":
    main()
