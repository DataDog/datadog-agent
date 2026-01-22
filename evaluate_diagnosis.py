#!/usr/bin/env python3
"""
Evaluate diagnosis accuracy against ground truth using OpenAI API.

Usage:
    export OPENAI_API_KEY="sk-..."
    python3 evaluate_diagnosis.py <diagnosis_file> --ground-truth <ground_truth_file>
    python3 evaluate_diagnosis.py <diagnosis_file> --scenario memory-leak
    python3 evaluate_diagnosis.py <diagnosis_file> --scenario network-latency

Scenarios (built-in ground truths):
    memory-leak      - Gradual memory leak causing OOM kills
    network-latency  - Network delay injected on Redis pod
"""

import json
import sys
import os
import argparse
import urllib.request
import urllib.error

# Map scenario names to human-readable problem types
PROBLEM_TYPES = {
    "memory-leak": "memory leak",
    "network-latency": "network latency",
}

# Built-in ground truths for common scenarios
GROUND_TRUTHS = {
    "memory-leak": """
## Ground Truth: Memory Leak Scenario

The injected fault is a gradual memory leak in a Python application that allocates memory without releasing it. The scenario runs a simple Python script that continuously allocates 512KB chunks every 2 seconds and stores them in a list to prevent garbage collection:

```python
# From deploy.yaml ConfigMap - the leaking application
leaked = []
chunk_size = 512 * 1024  # 512KB chunks

while True:
    leaked.append(bytearray(chunk_size))  # Allocate and retain memory
    time.sleep(2)
```

Container resource configuration:
```yaml
resources:
  requests:
    memory: "64Mi"
  limits:
    memory: "256Mi"  # Hard ceiling - triggers OOM at this point
```

**Root cause:** Application-level memory leak—the Python process accumulates heap allocations in a list (leaked.append(bytearray(...))) without ever releasing them, causing unbounded memory growth until the cgroup limit triggers an OOM kill.
""",

    "network-latency": """
## Ground Truth: Network Latency Scenario

The injected fault is artificial network latency on the Redis pod's network interface. The scenario uses Linux traffic control (tc) with the netem (network emulator) module to add 200ms ± 50ms of delay to all network packets on the Redis container's eth0 interface:

```python
# From scenario.py apply_network_latency() - lines 1392-1414
tc_result = subprocess.run(
    [
        "kubectl", "exec", "-n", namespace, pod_name, "--",
        "tc", "qdisc", "add", "dev", "eth0", "root", "netem",
        "delay", "200ms", "50ms",  # 200ms base delay ± 50ms jitter
    ],
    ...
)
```

**Root cause:** Network-level latency injection via tc qdisc netem on the Redis pod, simulating a network partition, congested link, or cross-datacenter communication delay.
"""
}

EVALUATION_PROMPT = """You are evaluating whether an automated diagnosis correctly identified the problem type.

## Ground Truth Problem Type: {problem_type}
{ground_truth}

## Diagnosis Output
{diagnosis}

## Evaluation Task

The key question: **Did the diagnosis identify that this is a {problem_type}?**

**Scoring (0-100):**
- **90-100**: Clearly and correctly identified it as a {problem_type}
- **70-89**: Mentioned {problem_type} or closely related terms (e.g., "memory growth", "OOM" for memory leak; "latency", "delay", "slow network" for network latency)
- **50-69**: Identified memory/network issues but was vague or mixed with other conclusions
- **30-49**: Mentioned the right general area but drew wrong conclusions
- **0-29**: Completely missed it, said "unclear", or diagnosed a different problem

**Respond with exactly this format:**

1. **Identified Problem**: [yes/partial/no] - Did it identify {problem_type}?
2. **Score**: [0-100]
3. **Evidence**: What terms/phrases in the diagnosis support your assessment? (one line)
4. **Summary**: One sentence assessment
"""


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


def evaluate(diagnosis_file, ground_truth, problem_type, model):
    api_key = os.environ.get('OPENAI_API_KEY')
    if not api_key:
        print("Error: Set OPENAI_API_KEY environment variable")
        print("  export OPENAI_API_KEY='sk-...'")
        sys.exit(1)
    
    # Read diagnosis file
    with open(diagnosis_file, 'r') as f:
        diagnosis = f.read()
    
    prompt = EVALUATION_PROMPT.format(
        ground_truth=ground_truth,
        diagnosis=diagnosis,
        problem_type=problem_type
    )
    
    print("=" * 60)
    print("EVALUATION")
    print("=" * 60)
    print(f"Diagnosis file: {diagnosis_file}")
    print(f"Problem type: {problem_type}")
    print(f"Model: {model}")
    print("=" * 60)
    print(f"\nChecking if diagnosis identified: {problem_type}...\n", flush=True)
    
    output = call_openai(prompt, model, api_key)
    
    print("=" * 60)
    print("EVALUATION RESULT:")
    print("=" * 60)
    print(output, flush=True)
    
    return output


def main():
    parser = argparse.ArgumentParser(
        description='Evaluate diagnosis accuracy against ground truth',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Use built-in ground truth
  python3 evaluate_diagnosis.py results.txt --scenario memory-leak
  python3 evaluate_diagnosis.py results.txt --scenario network-latency
  
  # Use custom ground truth file
  python3 evaluate_diagnosis.py results.txt --ground-truth my_ground_truth.txt
  
  # List available scenarios
  python3 evaluate_diagnosis.py --list-scenarios
"""
    )
    parser.add_argument('diagnosis_file', nargs='?', help='Path to diagnosis output file')
    parser.add_argument('--scenario', choices=list(GROUND_TRUTHS.keys()),
                        help='Use built-in ground truth for scenario')
    parser.add_argument('--ground-truth', dest='ground_truth_file',
                        help='Path to custom ground truth file')
    parser.add_argument('--problem-type', dest='problem_type',
                        help='Problem type to check for (required with --ground-truth)')
    parser.add_argument('--model', default='gpt-5.2-2025-12-11',
                        help='OpenAI model (default: gpt-5.2-2025-12-11)')
    parser.add_argument('--list-scenarios', action='store_true',
                        help='List available built-in scenarios')
    
    args = parser.parse_args()
    
    if args.list_scenarios:
        print("Available built-in scenarios:")
        for name, desc in GROUND_TRUTHS.items():
            # Get first line of description
            first_line = [l for l in desc.strip().split('\n') if l.strip()][0]
            print(f"  {name:20} - {first_line.strip('#').strip()}")
        sys.exit(0)
    
    if not args.diagnosis_file:
        parser.error("diagnosis_file is required (or use --list-scenarios)")
    
    if not args.scenario and not args.ground_truth_file:
        parser.error("Either --scenario or --ground-truth is required")
    
    if args.scenario and args.ground_truth_file:
        parser.error("Use either --scenario or --ground-truth, not both")
    
    # Get ground truth and problem type
    if args.scenario:
        ground_truth = GROUND_TRUTHS[args.scenario]
        problem_type = PROBLEM_TYPES[args.scenario]
    else:
        with open(args.ground_truth_file, 'r') as f:
            ground_truth = f.read()
        # For custom ground truth, require problem type
        if not args.problem_type:
            parser.error("--problem-type is required when using --ground-truth")
        problem_type = args.problem_type
    
    evaluate(args.diagnosis_file, ground_truth, problem_type, args.model)


if __name__ == '__main__':
    main()

