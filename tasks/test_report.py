"""Task to print a unified test output report from Go test JSON output."""

from invoke import task

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof import format_report
from tasks.libs.testing.utof.go_unit import convert_unit_test_results


@task
def print_report(ctx, file_path="test_output.json"):
    """
    Print a unified test output report from a Go test JSON output file.

    The input file should contain `go test -json` output, e.g.:
        {"Time":"...","Action":"run","Package":"...","Test":"TestFoo"}
        {"Time":"...","Action":"pass","Package":"...","Test":"TestFoo"}

    Usage:
        dda inv test-report.print-report --file-path=path/to/test_output.json
    """
    result = ResultJson.from_file(file_path)
    doc = convert_unit_test_results(ctx, result)
    print(format_report(doc))
