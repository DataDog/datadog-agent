#!/usr/bin/env python3
"""
Analyze observer demo results - compare detectors and correlators.
Usage: python3 analyze_results.py [pattern]
"""

import json
import glob
import sys
from collections import defaultdict

def load_results(pattern="*_v3.json"):
    """Load all result files matching the pattern."""
    results = []
    for filepath in sorted(glob.glob(pattern)):
        try:
            with open(filepath) as f:
                data = json.load(f)
                data['_filename'] = filepath
                results.append(data)
        except Exception as e:
            print(f"Warning: Failed to load {filepath}: {e}")
    return results

def get_edge_set(result):
    """Get set of edge pairs from a result (normalized so A-B == B-A)."""
    edges = set()
    for e in result.get('edges', []):
        pair = tuple(sorted([e.get('source1', ''), e.get('source2', '')]))
        edges.add(pair)
    return edges

def get_correlation_sources(result):
    """Get all sources mentioned in correlations."""
    sources = set()
    for c in result.get('correlations', []):
        for s in c.get('sources', []):
            sources.add(s)
    return sources

def print_header(title):
    print("\n" + "=" * 80)
    print(title)
    print("=" * 80)

def print_summary_table(results):
    """Print summary table."""
    print_header("SUMMARY TABLE")
    
    print(f"{'File':<45} {'Anomalies':>10} {'Unique Src':>12} {'Correlations':>12} {'Edges':>8}")
    print("-" * 90)
    
    for r in results:
        filename = r.get('_filename', '?')[:44]
        anomalies = r.get('total_anomalies', 0)
        unique_src = r.get('unique_sources_in_anomalies', 0)
        correlations = r.get('total_correlations', 0)
        edges = r.get('total_edges') or 0
        
        print(f"{filename:<45} {anomalies:>10,} {unique_src:>12} {correlations:>12} {edges:>8}")

def compare_edges(results):
    """Compare edges between GraphSketchCorrelator runs."""
    print_header("EDGE COMPARISON (GraphSketchCorrelator runs)")
    
    gs_results = [r for r in results if r.get('correlator') == 'GraphSketchCorrelator']
    
    if len(gs_results) < 2:
        print("Need at least 2 GraphSketchCorrelator runs to compare edges.")
        return
    
    # Get edges for each run
    edge_data = {}
    for r in gs_results:
        key = r.get('detector', 'unknown')
        edge_data[key] = {
            'edges': get_edge_set(r),
            'total': r.get('total_edges', 0),
            'result': r
        }
    
    detectors = list(edge_data.keys())
    
    for i, d1 in enumerate(detectors):
        for d2 in detectors[i+1:]:
            edges1 = edge_data[d1]['edges']
            edges2 = edge_data[d2]['edges']
            
            common = edges1 & edges2
            only_d1 = edges1 - edges2
            only_d2 = edges2 - edges1
            
            print(f"\n{d1} vs {d2}:")
            print(f"  {d1} total edges: {len(edges1)}")
            print(f"  {d2} total edges: {len(edges2)}")
            print(f"  ─────────────────────────────")
            print(f"  Common edges:     {len(common):>5} ({len(common)/max(len(edges1),1)*100:.1f}% of {d1})")
            print(f"  Only in {d1}:    {len(only_d1):>5}")
            print(f"  Only in {d2}: {len(only_d2):>5}")
            
            if common:
                # Calculate Jaccard similarity
                union = edges1 | edges2
                jaccard = len(common) / len(union) if union else 0
                print(f"  Jaccard similarity: {jaccard:.2%}")
            
            # Show ALL common edges
            if common:
                print(f"\n  ALL common edges ({len(common)}):")
                for edge in sorted(common):
                    print(f"    • {edge[0]} ↔ {edge[1]}")
            
            # Show ALL different edges
            if only_d1:
                print(f"\n  ALL edges only in {d1} ({len(only_d1)}):")
                for edge in sorted(only_d1):
                    print(f"    • {edge[0]} ↔ {edge[1]}")
            
            if only_d2:
                print(f"\n  ALL edges only in {d2} ({len(only_d2)}):")
                for edge in sorted(only_d2):
                    print(f"    • {edge[0]} ↔ {edge[1]}")

def compare_correlations(results):
    """Compare correlation contents between runs."""
    print_header("CORRELATION COMPARISON")
    
    # Group by correlator type
    by_correlator = defaultdict(list)
    for r in results:
        by_correlator[r.get('correlator', 'unknown')].append(r)
    
    for correlator, runs in by_correlator.items():
        print(f"\n{'─'*40}")
        print(f"{correlator}")
        print(f"{'─'*40}")
        
        # Show each run's correlations
        for r in runs:
            detector = r.get('detector', '?')
            correlations = r.get('correlations', [])
            total_corr = r.get('total_correlations', 0)
            
            print(f"\n  {detector}: {total_corr} correlation(s)")
            
            for c in correlations:  # Show all
                title = c.get('title', 'Unknown')
                source_count = c.get('source_count', len(c.get('sources', [])))
                sources = c.get('sources', [])
                
                print(f"    • {title}")
                print(f"      {source_count} sources: {', '.join(sources[:5])}{'...' if len(sources) > 5 else ''}")
        
        # Compare correlation sources between detectors
        if len(runs) >= 2:
            print(f"\n  Source overlap analysis:")
            
            for i, r1 in enumerate(runs):
                for r2 in runs[i+1:]:
                    d1, d2 = r1.get('detector'), r2.get('detector')
                    src1 = get_correlation_sources(r1)
                    src2 = get_correlation_sources(r2)
                    
                    if not src1 and not src2:
                        continue
                    
                    common = src1 & src2
                    only1 = src1 - src2
                    only2 = src2 - src1
                    union = src1 | src2
                    
                    jaccard = len(common) / len(union) if union else 0
                    
                    print(f"\n    {d1} vs {d2}:")
                    print(f"      {d1} sources in correlations: {len(src1)}")
                    print(f"      {d2} sources in correlations: {len(src2)}")
                    print(f"      Common sources: {len(common)}")
                    print(f"      Jaccard similarity: {jaccard:.2%}")
                    
                    if common:
                        print(f"      ALL common sources ({len(common)}):")
                        for s in sorted(common):
                            print(f"        • {s}")
                    if only1:
                        print(f"      ALL sources only in {d1} ({len(only1)}):")
                        for s in sorted(only1):
                            print(f"        • {s}")
                    if only2:
                        print(f"      ALL sources only in {d2} ({len(only2)}):")
                        for s in sorted(only2):
                            print(f"        • {s}")

def compare_unique_sources(results):
    """Compare unique sources detected by each detector."""
    print_header("UNIQUE SOURCES COMPARISON")
    
    # Group by detector
    by_detector = defaultdict(list)
    for r in results:
        by_detector[r.get('detector', 'unknown')].append(r)
    
    print("\nUnique sources per detector (should be consistent across correlators):")
    for detector, runs in sorted(by_detector.items()):
        counts = [r.get('unique_sources_in_anomalies', 0) for r in runs]
        correlators = [r.get('correlator', '?') for r in runs]
        
        print(f"\n  {detector}:")
        for corr, count in zip(correlators, counts):
            print(f"    {corr}: {count} unique sources")
        
        if len(set(counts)) == 1:
            print(f"    ✓ Consistent across correlators")
        else:
            print(f"    ⚠ INCONSISTENT - counts vary!")

def get_edge_observations(result, edge_pair):
    """Get observation count for a specific edge pair."""
    for e in result.get('edges', []):
        pair = tuple(sorted([e.get('source1', ''), e.get('source2', '')]))
        if pair == edge_pair:
            return e.get('observations', 0), e.get('frequency', 0)
    return 0, 0

def summary_recommendations(results):
    """Provide summary and insights about reliable edges."""
    print_header("RELIABLE EDGES ANALYSIS")
    
    gs_results = [r for r in results if r.get('correlator') == 'GraphSketchCorrelator']
    
    if len(gs_results) < 2:
        print("Need at least 2 GraphSketchCorrelator runs to find reliable edges.")
        return
    
    # Collect edges by detector with their stats
    edges_by_detector = {}
    for r in gs_results:
        detector = r.get('detector', 'unknown')
        edge_stats = {}
        for e in r.get('edges', []):
            pair = tuple(sorted([e.get('source1', ''), e.get('source2', '')]))
            edge_stats[pair] = {
                'observations': e.get('observations', 0),
                'frequency': e.get('frequency', 0)
            }
        edges_by_detector[detector] = edge_stats
    
    # Find edges common to ALL detectors
    all_edge_sets = [set(stats.keys()) for stats in edges_by_detector.values()]
    common_to_all = all_edge_sets[0].copy()
    for edge_set in all_edge_sets[1:]:
        common_to_all &= edge_set
    
    # Find edges unique to each detector
    all_edges = set()
    for edge_set in all_edge_sets:
        all_edges |= edge_set
    
    # Show ALL reliable edges with stats from each detector
    if common_to_all:
        print(f"\n{'='*60}")
        print(f"ALL RELIABLE EDGES (found by every detector) - {len(common_to_all)} edges")
        print(f"{'='*60}")
        print(f"\nThese edges represent metric pairs that co-occur during anomalies")
        print(f"regardless of which detection algorithm is used.\n")
        
        # Sort by total observations across detectors
        def total_obs(edge):
            return sum(edges_by_detector[d].get(edge, {}).get('observations', 0) 
                      for d in edges_by_detector)
        
        sorted_edges = sorted(common_to_all, key=total_obs, reverse=True)
        
        for i, edge in enumerate(sorted_edges, 1):
            print(f"\n{i}. {edge[0]} ↔ {edge[1]}")
            for detector, stats in edges_by_detector.items():
                if edge in stats:
                    obs = stats[edge]['observations']
                    freq = stats[edge]['frequency']
                    print(f"     {detector}: {obs:,} observations, freq={freq:.2f}")
        
        # Interpretation
        print(f"\n{'─'*60}")
        print("INTERPRETATION:")
        print("─"*60)
        print("""
These reliable edges indicate metrics that consistently anomaly together:
  • High observations = frequently co-occurring anomalies
  • Similar observations across detectors = stable relationship
  • Different observations = detector sensitivity differs for this pair

For a memory leak investigation, look for edges involving:
  • memory.* metrics paired with other system metrics
  • smaps_rollup.* (memory mapping) correlations
  • cgroup memory stats correlations
""")
    else:
        print("\n⚠ No edges found by ALL detectors!")
        print("This suggests the detectors are finding different patterns.")

def main():
    pattern = sys.argv[1] if len(sys.argv) > 1 else "*_v3.json"
    
    print(f"Loading results matching: {pattern}")
    results = load_results(pattern)
    
    if not results:
        print(f"No results found matching '{pattern}'")
        print("Run ./run_all_combos.sh first to generate results.")
        return
    
    print(f"Loaded {len(results)} result files")
    
    # Run analyses
    print_summary_table(results)
    compare_unique_sources(results)
    compare_edges(results)
    compare_correlations(results)
    summary_recommendations(results)
    
    print("\n" + "=" * 80)
    print("ANALYSIS COMPLETE")
    print("=" * 80 + "\n")

if __name__ == "__main__":
    main()
