from __future__ import annotations

from tasks.libs.common.color import Color, color_message
from tasks.libs.gpu.types import GPUConfigValidationResult, GPUConfigValidationState, ValidationResults


def color_status(status: GPUConfigValidationState) -> str:
    colors = {
        GPUConfigValidationState.OK: Color.GREEN,
        GPUConfigValidationState.FAIL: Color.RED,
        GPUConfigValidationState.MISSING: Color.ORANGE,
        GPUConfigValidationState.UNKNOWN: Color.ORANGE,
    }
    name = status.name.lower()
    return color_message(name, colors[status]) if status in colors else name


def color_metric_counts(missing: int, known: int, unknown: int) -> str:
    missing_str = color_message(str(missing), Color.RED) if missing > 0 else str(missing)
    known_str = color_message(str(known), Color.GREEN) if known > 0 else str(known)
    unknown_str = color_message(str(unknown), Color.ORANGE) if unknown > 0 else str(unknown)
    return f"{missing_str}/{known_str}/{unknown_str}"


def color_tag_failures(count: int) -> str:
    return color_message(str(count), Color.RED) if count > 0 else str(count)


def print_summary_table(title: str, results: list[GPUConfigValidationResult]) -> None:
    from tabulate import tabulate

    rows = [
        [
            row.config.architecture,
            row.config.device_mode,
            color_status(row.state),
            row.device_count,
            color_metric_counts(len(row.missing_metrics), len(row.present_metrics), len(row.unknown_metrics)),
            color_tag_failures(len(row.tag_failures)),
        ]
        for row in results
    ]

    print(f"\n{title}:")
    print(
        tabulate(
            rows,
            headers=[
                "architecture",
                "device mode",
                "status",
                "found devices",
                "missing/known/unknown metrics",
                "tag failures",
            ],
            tablefmt="github",
        )
    )


def print_result_details(results: list[GPUConfigValidationResult]) -> None:
    print("\nValidation details (showing only failures on configs with devices present):")
    for result in results:
        if not result.has_failures or result.device_count == 0:
            continue
        print(f"\n-- {result.config.architecture} {result.config.device_mode} --")
        print(f"  found devices: {result.device_count}")
        if result.missing_metrics:
            print("  missing metric names:")
            for name in result.missing_metrics:
                print(f"    - MISSING {name}")
        if result.unknown_metrics:
            print("  unknown metric names:")
            for name in result.unknown_metrics:
                print(f"    - UNKNOWN {name}")
        if result.tag_failures:
            print("  tag failure details:")
            for metric_name, tags in result.tag_failures.items():
                print(f"    - TAG FAIL {metric_name}: missing/non-null [{', '.join(tags)}]")


def render_results(result: ValidationResults) -> None:
    print(f"Loaded metrics spec: {result.metrics_count} entries")
    print(f"Loaded architecture spec: {result.architectures_count} architectures")
    print(f"Target site: {result.site}")
    print_summary_table("Summary", result.results)
    print_result_details(result.results)
    print(f"\nTotal combinations with metric/tag failures (and devices present): {result.failing_count}")
