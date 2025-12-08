#!/usr/bin/env python3
"""
Benchmark multiple SLMs on regex → token translation.

This script runs a small suite of regex patterns through one or more
small language models and compares:
  - The returned token sequence and prefilter keywords
  - How close they are to our expected "golden" outputs
  - Per-test latency per model

Usage (from repository root):

  python3 pkg/logs/processor/benchmark_slms.py \
    --models Qwen/Qwen2.5-Coder-3B \
    --json-only

You can pass multiple models to compare them:

  python3 pkg/logs/processor/benchmark_slms.py \
    --models Qwen/Qwen2.5-Coder-3B Qwen/Qwen2.5-Coder-1.5B

By default we only include Qwen2.5-Coder-3B; you can add any other
chat-style causal LM from Hugging Face as long as it supports the
`apply_chat_template` API.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import time
from dataclasses import dataclass
from typing import Any, Dict, List, Optional, Sequence

import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

from translate_regex import create_prompt


@dataclass
class TestCase:
    name: str
    regex: str
    description: str
    expected_tokens: Optional[List[str]] = None
    expected_prefilter: Optional[List[str]] = None


TEST_CASES: List[TestCase] = [
    # Simple, well-understood patterns
    TestCase(
        name="ssn",
        regex=r"\d{3}-\d{2}-\d{4}",
        description="US Social Security Number (123-45-6789)",
        expected_tokens=["D3", "Dash", "D2", "Dash", "D4"],
        expected_prefilter=["-"],
    ),
    TestCase(
        name="iso_date",
        regex=r"\d{4}-\d{2}-\d{2}",
        description="ISO date YYYY-MM-DD",
        expected_tokens=["D4", "Dash", "D2", "Dash", "D2"],
        expected_prefilter=["-"],
    ),
    # Medium complexity
    TestCase(
        name="ipv4",
        regex=r"\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}",
        description="IPv4 address",
        expected_tokens=["D1", "Period", "D1", "Period", "D1", "Period", "D1"],
        expected_prefilter=["."],
    ),
    TestCase(
        name="credit_card_16",
        regex=r"\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}",
        description="16-digit credit card with optional spaces/dashes",
        # There are many reasonable encodings; this is one space-separated example.
        expected_tokens=["D4", "Space", "D4", "Space", "D4", "Space", "D4"],
        expected_prefilter=[],
    ),
    TestCase(
        name="api_key_hex32",
        regex=r"[0-9a-fA-F]{32}",
        description="32-character hex API key",
        # This is intentionally loose; many strategies are reasonable.
        expected_tokens=["C32"],
        expected_prefilter=[],
    ),
    # Complex but common patterns
    TestCase(
        name="email",
        regex=r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}",
        description="Email address (simplified)",
        expected_tokens=["C1", "At", "C1", "Period", "C1"],
        expected_prefilter=["@", "."],
    ),
    TestCase(
        name="uuid",
        regex=r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}",
        description="UUID v4-like pattern",
        expected_tokens=[
            "C8",
            "Dash",
            "C4",
            "Dash",
            "C4",
            "Dash",
            "C4",
            "Dash",
            "C12",
        ],
        expected_prefilter=["-"],
    ),
    TestCase(
        name="iso_timestamp",
        regex=r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}",
        description="ISO8601 timestamp without timezone",
        expected_tokens=[
            "D4",
            "Dash",
            "D2",
            "Dash",
            "D2",
            "T",
            "D2",
            "Colon",
            "D2",
            "Colon",
            "D2",
        ],
        expected_prefilter=["T"],
    ),
    # Real-world exclude_at_match patterns from Kubernetes / Rails / infra logs.
    # For these we generally don't have a single canonical tokenization, so we
    # omit expected_* fields and just inspect the model output.
    TestCase(
        name="k8s_filter1",
        regex=r"mutation rules from policy applied successfully",
        description="Kubernetes admission controller success message",
    ),
    TestCase(
        name="k8s_filter2",
        regex=r"/ml/(ping|predict)",
        description="ML service ping/predict endpoints",
    ),
    TestCase(
        name="k8s_filter3",
        regex=r"churn-nginx-ingress-controller.*certificate",
        description="Ingress controller certificate logs",
    ),
    TestCase(
        name="k8s_filter4",
        regex=r'Service "churn/(chn|pur)-.*does not have any active Endpoint',
        description="Kubernetes service with no active endpoints",
    ),
    TestCase(
        name="k8s_filter5",
        regex=r"k8s_nginx-ingress-controller_nginx-ingress-controller",
        description="Ingress controller container name",
    ),
    TestCase(
        name="k8s_filter6",
        regex=r'Overriding "Content-Type" header "application/x-www-form-urlencoded"',
        description="Nginx overriding content-type header warning",
    ),
    TestCase(
        name="k8s_filter7",
        regex=r"URI.escape is obsolete",
        description="Ruby deprecation warning for URI.escape",
    ),
    TestCase(
        name="k8s_filter8",
        regex=r" warning: already initialized constant ",
        description="Ruby constant already-initialized warning",
    ),
    TestCase(
        name="telegraf_unsupported_distro",
        regex=r"Statsd Metric type d unsupported",
        description="Telegraf unsupported StatsD metric type",
    ),
    TestCase(
        name="rails_20X_ok",
        regex=r"Completed 20[012] .* in ",
        description="Rails 200/201/202 completion log line",
    ),
    TestCase(
        name="processing_by",
        regex=r"Processing by .*#.* as .*",
        description="Rails controller processing line",
    ),
    TestCase(
        name="rendered_text_template",
        regex=r"Rendered text template ",
        description="Rails rendered text template line",
    ),
]


def extract_json(response: str) -> Dict[str, Any]:
    """
    Extract the first JSON object from the model response.

    Mirrors the heuristic used in translate_regex.translate_regex.
    """
    try:
        json_match = re.search(r"\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}", response, re.DOTALL)
        if json_match:
            return json.loads(json_match.group())
        return {"error": "Could not parse JSON", "raw": response}
    except json.JSONDecodeError as exc:
        return {"error": f"JSON decode error: {exc}", "raw": response}


def translate_with_model(
    model_name: str,
    tokenizer: Any,
    model: Any,
    regex_pattern: str,
    description: str = "",
) -> Dict[str, Any]:
    """Run a single translation for a given model."""

    prompt = create_prompt(regex_pattern, description)

    messages = [
        {
            "role": "system",
            "content": (
                "You are a helpful coding assistant that translates regex patterns "
                "to token sequences. Output only valid JSON."
            ),
        },
        {"role": "user", "content": prompt},
    ]

    text = tokenizer.apply_chat_template(
        messages,
        tokenize=False,
        add_generation_prompt=True,
    )

    model_inputs = tokenizer([text], return_tensors="pt").to(model.device)

    start = time.time()
    generated_ids = model.generate(
        **model_inputs,
        max_new_tokens=512,
        temperature=0.3,
        top_p=0.9,
    )
    elapsed = time.time() - start

    # Strip the prompt tokens from the output
    generated_ids = [
        output_ids[len(input_ids) :]
        for input_ids, output_ids in zip(model_inputs.input_ids, generated_ids)
    ]

    response = tokenizer.batch_decode(generated_ids, skip_special_tokens=True)[0]
    result = extract_json(response)
    result["_raw_response"] = response
    result["_elapsed_seconds"] = elapsed
    result["_model_name"] = model_name
    return result


def list_equal(a: Optional[Sequence[str]], b: Optional[Sequence[str]]) -> bool:
    if a is None or b is None:
        return False
    return list(a) == list(b)


def main(argv: Optional[Sequence[str]] = None) -> None:
    parser = argparse.ArgumentParser(
        description="Benchmark regex→token translation across multiple SLMs.",
    )
    parser.add_argument(
        "--models",
        nargs="+",
        default=["Qwen/Qwen2.5-Coder-3B"],
        help=(
            "Hugging Face model IDs to benchmark. "
            "Example: Qwen/Qwen2.5-Coder-3B Qwen/Qwen2.5-Coder-1.5B"
        ),
    )
    parser.add_argument(
        "--json-only",
        action="store_true",
        help="Output raw JSON per (model, test case) instead of a human summary.",
    )

    args = parser.parse_args(argv)

    all_results: List[Dict[str, Any]] = []

    for model_name in args.models:
        print("=" * 80)
        print(f"Loading model: {model_name}")
        print("=" * 80)

        try:
            tokenizer = AutoTokenizer.from_pretrained(model_name)
            model = AutoModelForCausalLM.from_pretrained(
                model_name,
                torch_dtype="auto",
                device_map="auto",
            )
        except Exception as exc:  # pylint: disable=broad-except
            print(f"Error loading model {model_name}: {exc}", file=sys.stderr)
            continue

        for tc in TEST_CASES:
            print()
            print("-" * 80)
            print(f"Model: {model_name}")
            print(f"Test : {tc.name}")
            print(f"Regex: {tc.regex}")
            print("-" * 80)

            result = translate_with_model(
                model_name=model_name,
                tokenizer=tokenizer,
                model=model,
                regex_pattern=tc.regex,
                description=tc.description,
            )

            tokens = result.get("tokens")
            prefilter = result.get("prefilter_keywords")
            elapsed = result.get("_elapsed_seconds", 0.0)

            if tc.expected_tokens is not None:
                match_tokens = list_equal(tokens, tc.expected_tokens)
            else:
                match_tokens = None

            if tc.expected_prefilter is not None:
                match_prefilter = list_equal(prefilter, tc.expected_prefilter)
            else:
                match_prefilter = None

            result["_test_name"] = tc.name
            result["_regex"] = tc.regex
            result["_expected_tokens"] = tc.expected_tokens
            result["_expected_prefilter"] = tc.expected_prefilter
            result["_match_tokens"] = match_tokens
            result["_match_prefilter"] = match_prefilter

            all_results.append(result)

            if args.json_only:
                print(json.dumps(result, indent=2))
            else:
                print(f"Elapsed: {elapsed * 1000.0:.1f} ms")
                print(f"Tokens : {tokens}")
                print(f"Prefilter: {prefilter}")
                if tc.expected_tokens is not None:
                    print(f"Expected tokens   : {tc.expected_tokens}")
                    print(f"Tokens exact match: {match_tokens}")
                if tc.expected_prefilter is not None:
                    print(f"Expected prefilter: {tc.expected_prefilter}")
                    print(f"Prefilter match   : {match_prefilter}")
                if result.get("explanation"):
                    print(f"Explanation: {result['explanation']}")
                if result.get("notes"):
                    print(f"Notes      : {result['notes']}")

    if args.json_only:
        # When json-only, also dump a final JSON array on stdout so it can be
        # post-processed easily.
        print()
        print(json.dumps(all_results, indent=2))


if __name__ == "__main__":
    main()


