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

    # Write Excel
    write_excel(results, output_file)

    print(f"\nTo use in Google Sheets:")
    print(f"1. Open Google Sheets")
    print(f"2. File > Import > Upload")
    print(f"3. Select: {output_file}")

    return 0


if __name__ == "__main__":
    sys.exit(main())
