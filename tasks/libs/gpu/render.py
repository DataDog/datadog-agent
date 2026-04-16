from __future__ import annotations

from tasks.libs.common.color import Color, color_message
from tasks.libs.gpu.types import GPUConfigValidationResult, GPUConfigValidationState, ValidationResults

SPACER = "  "


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
            color_metric_counts(row.missing_metrics, row.present_metrics, row.unknown_metrics),
            color_tag_failures(row.tag_failures),
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
        if result.state is not GPUConfigValidationState.FAIL or result.device_count == 0:
            continue

        print(f"\n-- {result.config.architecture} {result.config.device_mode} --")
        print(f"{SPACER}found devices: {result.device_count}")
        print(f"{SPACER}summary")
        print(f"{SPACER * 2}missing={result.missing_metrics}")
        print(f"{SPACER * 2}known={result.present_metrics}")
        print(f"{SPACER * 2}unknown={result.unknown_metrics}")
        print(f"{SPACER * 2}tag failures={result.tag_failures}")

        failing_metrics = [
            (metric_name, metric_status)
            for metric_name, metric_status in sorted(result.detailed_result.metrics.items())
            if metric_status.has_failures
        ]

        if failing_metrics:
            print(f"{SPACER}metric failures")

            for metric_name, metric_status in failing_metrics:
                print(f"{SPACER * 2}- {metric_name}: {', '.join(metric_status.errors)}")

                for tag_name, tag_result in sorted(metric_status.tag_results.items()):
                    if not tag_result.has_failures:
                        continue
                    details: list[str] = []
                    if tag_result.missing > 0:
                        total = tag_result.found + tag_result.missing
                        missing_rate = 0.0 if total == 0 else (tag_result.missing / total) * 100
                        details.append(f"missing {tag_result.missing}/{total} ({missing_rate:.1f}%)")
                    if tag_result.unknown > 0:
                        details.append(f"unknown={tag_result.unknown}")
                    if tag_result.invalid_value > 0:
                        invalid_detail = f"invalid={tag_result.invalid_value}"
                        if tag_result.invalid_value_samples:
                            invalid_detail += f" samples=[{', '.join(tag_result.invalid_value_samples)}]"
                        details.append(invalid_detail)
                    if not details:
                        continue
                    print(f"{SPACER * 3}- tag {tag_name}: {', '.join(details)}")


def render_results(result: ValidationResults) -> None:
    print(f"Loaded metrics spec: {result.metrics_count} entries")
    print(f"Loaded architecture spec: {result.architectures_count} architectures")
    print(f"Target site: {result.site}")
    print_summary_table("Summary", result.results)
    print_result_details(result.results)
    print(f"\nTotal combinations with metric/tag failures (and devices present): {result.failing_count}")
