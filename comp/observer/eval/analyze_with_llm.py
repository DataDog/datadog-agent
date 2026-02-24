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

import json
import sys
import os
import argparse
import urllib.request
import urllib.error

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
- rca (optional): ranked root candidates + evidence paths + confidence flags
- sample_anomalies: A few example anomalies for context
"""

def build_digest_hint(parsed):
    """Build a structured hint from correlation digests and RCA results.

    The hint varies based on RCA confidence:
    - High confidence (>= 0.6): Includes RCA-ranked key sources, onset chain,
      and causal framing to guide the LLM toward likely root causes.
    - Low confidence (< 0.6): Includes only metric family breadth and
      representative samples. No causal assertions — lets the LLM reason from
      the structural summary without being misled by uncertain rankings.
    """
    if not isinstance(parsed, dict):
        return ""

    HIGH_CONF_THRESHOLD = 0.6
    lines = []

    # Check for digests on correlations.
    correlations = parsed.get("correlations", [])
    has_digest = False
    for corr in correlations:
        if not isinstance(corr, dict):
            continue
        digest = corr.get("digest")
        if not isinstance(digest, dict):
            continue
        has_digest = True
        pattern = corr.get("pattern", "unknown")
        total = digest.get("total_source_count", 0)
        conf = digest.get("rca_confidence", 0)
        flags = digest.get("confidence_flags", [])
        flags_str = ", ".join(flags) if flags else "none"
        high_confidence = conf >= HIGH_CONF_THRESHOLD

        lines.append(f"**Correlation {pattern}** ({total} anomalous series, RCA confidence: {conf:.2f}, flags: {flags_str})")

        # Metric family summary (sorted by count, top 15) — always included.
        families = digest.get("metric_family_counts", {})
        if families:
            sorted_fam = sorted(families.items(), key=lambda x: -x[1])[:15]
            fam_strs = [f"{name}: {count}" for name, count in sorted_fam]
            remaining = len(families) - len(sorted_fam)
            lines.append(f"  Metric families: {', '.join(fam_strs)}")
            if remaining > 0:
                lines.append(f"  ...and {remaining} more metric families")

        if high_confidence:
            # --- High-confidence mode: causal detail ---
            # Temporal onset chain.
            onset_chain = digest.get("onset_chain", [])
            if onset_chain:
                chain_parts = []
                for entry in onset_chain:
                    if isinstance(entry, dict):
                        metric = entry.get("metric_name", "?")
                        t = entry.get("onset_time", 0)
                        chain_parts.append(f"{metric} (T={t})")
                if chain_parts:
                    lines.append(f"  Onset chain: {' -> '.join(chain_parts)}")

            # Key sources (top-ranked by RCA).
            key_sources = digest.get("key_sources", [])
            if key_sources:
                lines.append(f"  Top {len(key_sources)} ranked root-cause candidates:")
                for ks in key_sources:
                    if not isinstance(ks, dict):
                        continue
                    metric = ks.get("metric_name", "?")
                    score = ks.get("score", 0)
                    why = ks.get("why", [])
                    why_str = "; ".join(why) if why else ""
                    line = f"    - {metric} (score={score:.3f})"
                    if why_str:
                        line += f" — {why_str}"
                    lines.append(line)
        else:
            # --- Low-confidence mode: structural summary only ---
            lines.append(f"  NOTE: RCA confidence is low ({conf:.2f}). The causal ordering is uncertain.")
            lines.append(f"  Focus on the metric family breadth and overall anomaly pattern rather than specific root-cause rankings.")

            # Representative samples (no scores, no causal framing).
            key_sources = digest.get("key_sources", [])
            if key_sources:
                lines.append(f"  Representative series from most impacted families:")
                for ks in key_sources:
                    if not isinstance(ks, dict):
                        continue
                    metric = ks.get("metric_name", "?")
                    why = ks.get("why", [])
                    why_str = "; ".join(why) if why else ""
                    line = f"    - {metric}"
                    if why_str:
                        line += f" ({why_str})"
                    lines.append(line)

        lines.append("")

    # Include RCA root candidates/evidence paths only when high-confidence.
    rca_results = parsed.get("rca", [])
    if isinstance(rca_results, list) and rca_results:
        for result in rca_results[:3]:
            if not isinstance(result, dict):
                continue
            conf_obj = result.get("confidence", {})
            conf_score = conf_obj.get("score", 0) if isinstance(conf_obj, dict) else 0
            if conf_score < HIGH_CONF_THRESHOLD:
                continue  # Skip low-confidence RCA detail

            pattern = result.get("correlation_pattern", "unknown")

            top_metric = "none"
            roots_metric = result.get("root_candidates_metric")
            if isinstance(roots_metric, list) and roots_metric:
                root = roots_metric[0]
                if isinstance(root, dict):
                    top_metric = f"{root.get('id', 'unknown')} (score={root.get('score', 'n/a')})"

            evidence_path = ""
            paths = result.get("evidence_paths")
            if isinstance(paths, list) and paths:
                first = paths[0]
                if isinstance(first, dict) and isinstance(first.get("nodes"), list):
                    evidence_path = " -> ".join(str(x) for x in first["nodes"][:6])

            lines.append(f"  RCA {pattern}: top_metric={top_metric}")
            if evidence_path:
                lines.append(f"    evidence path: {evidence_path}")

    if not lines:
        return ""

    header = "**Correlation Digest** (compressed from raw sources using structural analysis):"
    return header + "\n" + "\n".join(lines)


def build_rca_hint(parsed):
    """Build a compact RCA summary for the prompt when available.

    Kept as fallback for outputs without digests (e.g. non-RCA runs).
    """
    if not isinstance(parsed, dict):
        return ""
    rca_results = parsed.get("rca")
    if not isinstance(rca_results, list) or not rca_results:
        return ""

    lines = ["RCA evidence (present in data):"]
    for result in rca_results[:3]:
        if not isinstance(result, dict):
            continue
        pattern = result.get("correlation_pattern", "unknown")

        top_series = "none"
        roots_series = result.get("root_candidates_series")
        if isinstance(roots_series, list) and roots_series:
            root = roots_series[0]
            if isinstance(root, dict):
                top_series = f"{root.get('id', 'unknown')} (score={root.get('score', 'n/a')})"

        top_metric = "none"
        roots_metric = result.get("root_candidates_metric")
        if isinstance(roots_metric, list) and roots_metric:
            root = roots_metric[0]
            if isinstance(root, dict):
                top_metric = f"{root.get('id', 'unknown')} (score={root.get('score', 'n/a')})"

        conf = result.get("confidence", {})
        conf_score = conf.get("score", "n/a") if isinstance(conf, dict) else "n/a"
        flags = []
        if isinstance(conf, dict):
            if conf.get("data_limited"):
                flags.append("data_limited")
            if conf.get("weak_directionality"):
                flags.append("weak_directionality")
            if conf.get("ambiguous_roots"):
                flags.append("ambiguous_roots")
        flags_str = ",".join(flags) if flags else "none"

        evidence_path = ""
        paths = result.get("evidence_paths")
        if isinstance(paths, list) and paths:
            first = paths[0]
            if isinstance(first, dict) and isinstance(first.get("nodes"), list):
                evidence_path = " -> ".join(str(x) for x in first["nodes"][:6])

        lines.append(
            f"- {pattern}: top_series={top_series}; top_metric={top_metric}; conf={conf_score}; flags={flags_str}"
        )
        if evidence_path:
            lines.append(f"  path: {evidence_path}")

    return "\n".join(lines)

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
    
    payload = json.dumps({
        "model": model,
        "messages": [{"role": "user", "content": prompt}]
    }).encode('utf-8')
    
    headers = {
        'Content-Type': 'application/json',
        'Authorization': f'Bearer {api_key}'
    }
    
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
    
    with open(filepath, 'r') as f:
        data = f.read()

    parsed = None
    try:
        parsed = json.loads(data)
    except json.JSONDecodeError:
        parsed = None

    # Prefer digest hint (structured summary) over raw RCA hint.
    analysis_hint = build_digest_hint(parsed)
    if not analysis_hint:
        analysis_hint = build_rca_hint(parsed)

    rca_instruction = ""
    if analysis_hint:
        rca_instruction = "Use the correlation digest to focus your analysis. When RCA confidence is high, trust the ranked root-cause candidates and onset chain. When RCA confidence is low (noted in the digest), do NOT rely on specific series rankings for causation — instead focus on the overall metric family breadth, anomaly counts, and system-wide patterns to form your own hypothesis."

    prompt = f"""{context}
{analysis_hint}

Here is the data:

{data}

Be concise. Answer:
1. What do the correlations tell you?
2. Is there a problem? (yes/no/unclear)
3. If yes, what is it? (one sentence)
4. Confidence level (high/medium/low)
5. If not high confidence: what are the alternative possibilities and why are you uncertain?
6. Supporting evidence (bullet points from the data)
7. If RCA/digest is present: explicitly state whether the key sources and onset chain support your conclusion.
{rca_instruction}"""
    
    print("="*60)
    print("PROMPT (context only, JSON data omitted):")
    print("="*60)
    print(context)
    print("\n[JSON data omitted]\n")
    print("Be concise. Answer: 1. What do correlations mean? 2. Problem? 3. What? 4. Confidence 5. If uncertain, alternatives? 6. Evidence")
    print("="*60)
    print(f"\nAnalyzing with {model}...\n", flush=True)
    
    output = call_openai(prompt, model, api_key)
    print("="*60)
    print("RESPONSE:")
    print("="*60)
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
