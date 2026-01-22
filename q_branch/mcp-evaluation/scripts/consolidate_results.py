#!/usr/bin/env python3
"""
Consolidate evaluation results from JSONL files into an Excel workbook.

Creates a multi-sheet Excel file with:
- Summary sheet: Aggregate statistics per mode
- Individual sheets per mode: Detailed results for each scenario

Usage:
    python consolidate_results.py [run_directory]

If run directory is not specified, uses the latest run in results/
Google Sheets can import the generated Excel file directly.
"""

import sys
import json
from pathlib import Path
from typing import Dict, List, Any
import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
from openpyxl.drawing.image import Image


def get_latest_run_dir(results_dir: Path) -> Path:
    """Get the most recent run directory"""
    run_dirs = sorted(results_dir.glob("run-*"), reverse=True)
    if not run_dirs:
        raise FileNotFoundError(f"No run directories found in {results_dir}")
    return run_dirs[0]


def load_results(run_dir: Path) -> List[Dict[str, Any]]:
    """Load all JSONL result files from the run directory"""
    all_results = []

    for jsonl_file in run_dir.glob("evaluation-*.jsonl"):
        print(f"Loading {jsonl_file.name}...")
        with open(jsonl_file, 'r') as f:
            for line in f:
                if line.strip():
                    result = json.loads(line)
                    all_results.append(result)

    return all_results


def flatten_result(result: Dict[str, Any]) -> Dict[str, Any]:
    """Flatten nested result structure for tabular export (no category scores)"""
    flattened = {
        'mode': result.get('mode'),
        'scenario': result.get('scenario'),
        'status': result.get('status'),
        'overall_score': result.get('score', {}).get('overall_score'),
        'strengths': '; '.join(result.get('score', {}).get('strengths', [])),
        'weaknesses': '; '.join(result.get('score', {}).get('weaknesses', [])),
        'key_terms_found': ', '.join(result.get('score', {}).get('key_terms_found', [])),
        'key_terms_missing': ', '.join(result.get('score', {}).get('key_terms_missing', [])),
        'error': result.get('error', '').replace('\n', ' ') if result.get('error') else None,
        'duration_sec': round(result.get('duration_ms', 0) / 1000, 2) if result.get('duration_ms') else None,
        'turns': result.get('turns'),
        'cost': result.get('cost'),
        'timestamp': result.get('timestamp'),
    }

    return flattened


def create_summary_stats(df: pd.DataFrame) -> pd.DataFrame:
    """Create summary statistics per mode"""
    # Filter to completed results only for stats
    completed = df[df['status'] == 'completed'].copy()

    # Group by mode and calculate aggregates
    summary = completed.groupby('mode').agg({
        'overall_score': ['mean', 'min', 'max'],
        'duration_sec': 'mean',
        'turns': 'mean',
        'cost': ['mean', 'sum'],
        'scenario': 'count'  # Count of completed scenarios
    }).round(2)

    # Flatten multi-level columns
    summary.columns = ['avg_score', 'min_score', 'max_score', 'avg_duration_sec', 'avg_turns', 'avg_cost', 'total_cost', 'completed_count']

    # Add error count
    errors = df[df['status'] == 'error'].groupby('mode').size()
    summary['error_count'] = errors
    summary['error_count'] = summary['error_count'].fillna(0).astype(int)

    # Add total count
    totals = df.groupby('mode').size()
    summary['total_scenarios'] = totals

    # Reset index to make mode a column
    summary = summary.reset_index()

    # Reorder columns
    summary = summary[[
        'mode', 'total_scenarios', 'completed_count', 'error_count',
        'avg_score', 'min_score', 'max_score',
        'avg_duration_sec', 'avg_turns', 'avg_cost', 'total_cost'
    ]]

    return summary


def create_score_comparison_chart(summary_sheet, results: List[Dict[str, Any]], output_file: Path):
    """Create a grouped bar chart as PNG and embed it in the summary sheet"""
    if not results:
        print("No results to create chart")
        return

    # Flatten results and create dataframe
    flattened_results = [flatten_result(r) for r in results]
    df = pd.DataFrame(flattened_results)

    # Filter to completed results only
    df = df[df['status'] == 'completed'].copy()

    if df.empty:
        print("No completed results to chart")
        return

    # Pivot data: scenarios as rows, modes as columns, scores as values
    pivot = df.pivot(index='scenario', columns='mode', values='overall_score')

    # Sort scenarios alphabetically
    pivot = pivot.sort_index()

    # Get modes in consistent order
    mode_order = ['bash', 'safe-shell', 'tools']
    available_modes = [m for m in mode_order if m in pivot.columns]
    pivot = pivot[available_modes]

    # Create figure and axis
    fig, ax = plt.subplots(figsize=(12, 6))

    # Set up bar positions
    scenarios = pivot.index.tolist()
    x = np.arange(len(scenarios))
    width = 0.25

    # Colors for each mode
    colors = {
        'bash': '#1f77b4',         # Blue
        'safe-shell': '#ff7f0e',   # Orange
        'tools': '#2ca02c'         # Green
    }

    # Create bars for each mode
    for i, mode in enumerate(available_modes):
        offset = width * (i - len(available_modes)/2 + 0.5)
        scores = pivot[mode].values
        bars = ax.bar(x + offset, scores, width,
                     label=mode, color=colors.get(mode, '#333333'),
                     edgecolor='black', linewidth=0.5)

    # Customize chart
    ax.set_xlabel('Scenario', fontsize=14, fontweight='bold')
    ax.set_ylabel('Score (0-100)', fontsize=14, fontweight='bold')
    ax.set_title('Diagnostic Score Comparison Across Modes and Scenarios',
                fontsize=16, fontweight='bold', pad=20)
    ax.set_xticks(x)
    ax.set_xticklabels(scenarios, rotation=45, ha='right', fontsize=10)
    ax.set_ylim(0, 105)
    ax.set_yticks(range(0, 101, 10))
    ax.legend(title='Mode', loc='upper left', bbox_to_anchor=(1.02, 1), fontsize=12)
    ax.grid(axis='y', alpha=0.3, linestyle='--')

    # Tight layout to prevent label cutoff
    plt.tight_layout()

    # Save figure as PNG
    chart_file = output_file.parent / "score_comparison.png"
    plt.savefig(chart_file, dpi=100, bbox_inches='tight')
    plt.close()

    print(f"Score comparison chart saved to {chart_file}")

    # Embed image in Excel sheet
    img = Image(str(chart_file))
    # Scale the image to 75% of original size
    img.width = img.width * 0.75
    img.height = img.height * 0.75
    # Position below the summary table
    summary_sheet.add_image(img, 'A10')

    print("Embedded chart in Summary sheet")


def write_excel(results: List[Dict[str, Any]], output_file: Path):
    """Write results to Excel workbook with multiple sheets"""
    if not results:
        print("No results to write")
        return

    # Flatten all results
    flattened_results = [flatten_result(r) for r in results]
    df = pd.DataFrame(flattened_results)

    # Sort by mode, then scenario
    df = df.sort_values(['mode', 'scenario']).reset_index(drop=True)

    # Create Excel writer
    with pd.ExcelWriter(output_file, engine='openpyxl') as writer:
        # Write summary sheet
        print("\nCreating Summary sheet...")
        summary = create_summary_stats(df)
        summary.to_excel(writer, sheet_name='Summary', index=False)

        # Write individual sheets per mode
        for mode in sorted(df['mode'].unique()):
            print(f"Creating {mode} sheet...")
            mode_df = df[df['mode'] == mode].copy()

            # Drop mode column for individual sheets (redundant)
            mode_df = mode_df.drop('mode', axis=1)

            # Write to sheet
            mode_df.to_excel(writer, sheet_name=mode, index=False)

            # Auto-adjust column widths and format currency columns
            worksheet = writer.sheets[mode]
            for idx, col in enumerate(mode_df.columns):
                max_length = max(
                    mode_df[col].astype(str).apply(len).max(),
                    len(col)
                )
                # Cap at 60 for readability
                worksheet.column_dimensions[chr(65 + idx)].width = min(max_length + 2, 60)

                # Format cost column as USD currency
                if col == 'cost':
                    col_letter = chr(65 + idx)
                    for row in range(2, len(mode_df) + 2):  # Start from row 2 (skip header)
                        cell = worksheet[f'{col_letter}{row}']
                        cell.number_format = '$#,##0.00'

        # Auto-adjust summary sheet column widths and format currency columns
        summary_sheet = writer.sheets['Summary']
        for idx, col in enumerate(summary.columns):
            max_length = max(
                summary[col].astype(str).apply(len).max(),
                len(col)
            )
            summary_sheet.column_dimensions[chr(65 + idx)].width = max_length + 2

            # Format cost columns as USD currency
            if col in ('avg_cost', 'total_cost'):
                col_letter = chr(65 + idx)
                for row in range(2, len(summary) + 2):  # Start from row 2 (skip header)
                    cell = summary_sheet[f'{col_letter}{row}']
                    cell.number_format = '$#,##0.00'

        # Add score comparison chart to summary sheet
        print("Creating Score Comparison chart...")
        create_score_comparison_chart(summary_sheet, results, output_file)

    print(f"\nConsolidated {len(flattened_results)} results to {output_file}")

    # Print summary to console
    print("\n" + "="*80)
    print("SUMMARY")
    print("="*80)
    print(summary.to_string(index=False))
    print("="*80)


def main():
    # Determine paths
    script_dir = Path(__file__).parent
    mcp_eval_dir = script_dir.parent
    results_base_dir = mcp_eval_dir / "results"

    # Determine run directory
    if len(sys.argv) > 1:
        run_dir = Path(sys.argv[1])
        if not run_dir.is_absolute():
            run_dir = results_base_dir / run_dir
    else:
        print("No run directory specified, using latest run...")
        run_dir = get_latest_run_dir(results_base_dir)

    if not run_dir.exists():
        print(f"Error: Run directory does not exist: {run_dir}")
        return 1

    # Output file goes in the run directory
    output_file = run_dir / "results.xlsx"

    print(f"Run directory: {run_dir}")
    print(f"Output file: {output_file}\n")

    # Load and consolidate
    results = load_results(run_dir)

    if not results:
        print(f"No evaluation results found in {run_dir}")
        return 1

    # Write Excel (includes score comparison chart)
    write_excel(results, output_file)

    print(f"\nTo use in Google Sheets:")
    print(f"1. Open Google Sheets")
    print(f"2. File > Import > Upload")
    print(f"3. Select: {output_file}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
