#!/usr/bin/env python3
"""
Analyze observer results using OpenAI API (no dependencies).

Usage:
    export OPENAI_API_KEY="sk-..."
    python3 analyze_with_llm.py <json_file> [flags]

Flags:
    --cusum         Add context about CUSUM detector
    --lightesd      Add context about LightESD detector
    --graphsketch   Add context about GraphSketch correlator
    --timecluster   Add context about TimeCluster correlator
    --all           Add all context (default if no flags)
    --model         Model to use (default: gpt-5.2-2025-12-11)
"""

import argparse
import json
import os
import sys
import urllib.error
import urllib.request

BASE_CONTEXT = """This is anomaly detection output from Linux container metrics (cgroup v2, smaps, CPU, memory, I/O).

The data was processed by two components:
1. An anomaly detector (identifies abnormal metric values)
2. A correlator (groups anomalies that occur together)
"""

CUSUM_CONTEXT = """
**CUSUM Detector:**
Detects sustained shifts from baseline using cumulative sum of deviations.
- Learns "normal" from first 25% of data
- Accumulates "debt" when values deviate from normal
- Triggers when cumulative deviation exceeds threshold
- Good for: gradual changes, memory leaks, sustained CPU increases
- Not good for: brief spikes that quickly return to normal
"""

LIGHTESD_CONTEXT = """
**LightESD Detector:**
Detects statistical outliers using robust methods.
- Removes trend (if values generally go up/down)
- Removes seasonality (hourly/daily patterns)
- Finds points statistically far from the median
- Uses robust stats (median, MAD) to avoid being fooled by outliers
- Good for: sudden spikes, brief anomalies in noisy data
- Not good for: gradual shifts
"""

GRAPHSKETCH_CONTEXT = """
**GraphSketch Correlator:**
Learns which metrics frequently anomaly together over time.
- Tracks "edges" (pairs of metrics that co-occur within 10 seconds)
- Counts co-occurrence frequency with time decay (recent matters more)
- Reports edges with observation counts and frequency scores
- Good for: finding root causes, understanding metric relationships
- Slower processing, learns patterns over time
"""

TIMECLUSTER_CONTEXT = """
**TimeCluster Correlator:**
Groups metrics that anomaly at the exact same time (within 1 second).
- Simple temporal grouping, no learning
- Creates clusters of simultaneous anomalies
- Good for: quick grouping during incidents
- Fast processing, but doesn't understand relationships
"""

JSON_CONTEXT = """
**JSON fields:**
- total_anomalies: How many anomaly events were detected
- unique_sources_in_anomalies: How many different metrics had anomalies
- correlations: Groups of metrics that anomalied together
- edges (if GraphSketch): Pairs of metrics and how often they co-occurred
- sample_anomalies: A few example anomalies for context
"""


def build_context(args):
    """Build context string based on flags."""
    parts = [BASE_CONTEXT]

    include_all = args.all or not (args.cusum or args.lightesd or args.graphsketch or args.timecluster)

    if args.cusum or include_all:
        parts.append(CUSUM_CONTEXT)
    if args.lightesd or include_all:
        parts.append(LIGHTESD_CONTEXT)
    if args.graphsketch or include_all:
        parts.append(GRAPHSKETCH_CONTEXT)
    if args.timecluster or include_all:
        parts.append(TIMECLUSTER_CONTEXT)

    parts.append(JSON_CONTEXT)
    return "\n".join(parts)


def call_openai(prompt, model, api_key):
    """Call OpenAI API using urllib (no dependencies)."""
    url = "https://api.openai.com/v1/chat/completions"

    payload = json.dumps({"model": model, "messages": [{"role": "user", "content": prompt}]}).encode('utf-8')

    headers = {'Content-Type': 'application/json', 'Authorization': f'Bearer {api_key}'}

    req = urllib.request.Request(url, data=payload, headers=headers)

    try:
        with urllib.request.urlopen(req) as response:
            result = json.loads(response.read().decode('utf-8'))
            return result['choices'][0]['message']['content']
    except urllib.error.HTTPError as e:
        error_body = e.read().decode('utf-8')
        print(f"OpenAI API error ({e.code}): {error_body}")
        sys.exit(1)
    except urllib.error.URLError as e:
        print(f"Connection error: {e}")
        sys.exit(1)


def analyze(filepath, context, model):
    api_key = os.environ.get('OPENAI_API_KEY')
    if not api_key:
        print("Error: Set OPENAI_API_KEY environment variable")
        print("  export OPENAI_API_KEY='sk-...'")
        sys.exit(1)

    with open(filepath) as f:
        data = f.read()

    prompt = f"""{context}

Here is the data:

{data}

Be concise. Answer:
1. What do the correlations tell you?
2. Is there a problem? (yes/no/unclear)
3. If yes, what is it? (one sentence)
4. Confidence level (high/medium/low)
5. If not high confidence: what are the alternative possibilities and why are you uncertain?
6. Supporting evidence (bullet points from the data)"""

    print("=" * 60)
    print("PROMPT (context only, JSON data omitted):")
    print("=" * 60)
    print(context)
    print("\n[JSON data omitted]\n")
    print(
        "Be concise. Answer: 1. What do correlations mean? 2. Problem? 3. What? 4. Confidence 5. If uncertain, alternatives? 6. Evidence"
    )
    print("=" * 60)
    print(f"\nAnalyzing with {model}...\n", flush=True)

    output = call_openai(prompt, model, api_key)
    print("=" * 60)
    print("RESPONSE:")
    print("=" * 60)
    print(output, flush=True)


def main():
    parser = argparse.ArgumentParser(description='Analyze observer results with OpenAI')
    parser.add_argument('json_file', help='Path to JSON results file')
    parser.add_argument('--cusum', action='store_true', help='Add CUSUM detector context')
    parser.add_argument('--lightesd', action='store_true', help='Add LightESD detector context')
    parser.add_argument('--graphsketch', action='store_true', help='Add GraphSketch correlator context')
    parser.add_argument('--timecluster', action='store_true', help='Add TimeCluster correlator context')
    parser.add_argument('--all', action='store_true', help='Add all context (default)')
    parser.add_argument('--model', default='gpt-5.2-2025-12-11', help='OpenAI model (default: gpt-5.2-2025-12-11)')

    args = parser.parse_args()

    context = build_context(args)
    analyze(args.json_file, context, args.model)


if __name__ == '__main__':
    main()
