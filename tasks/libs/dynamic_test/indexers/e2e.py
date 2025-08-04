import json
from pathlib import Path

from invoke import Context

from tasks.libs.common.color import Color, color_message
from tasks.libs.dynamic_test.index import DynamicTestIndex
from tasks.libs.dynamic_test.indexer import DynTestIndexer


class CoverageDynTestIndexer(DynTestIndexer):
    """Coverage-based indexer.

    Walks a coverage output root directory, converts each suite's coverage to text, parses
    covered packages, and builds a reverse index of job->package->[tests].
    """

    def __init__(self, coverage_root: str) -> None:
        self.coverage_root = coverage_root

    def compute_index(self, ctx: Context) -> DynamicTestIndex:
        root = Path(self.coverage_root)
        if not root.exists() or not root.is_dir():
            raise FileNotFoundError(f"Coverage root not found or not a directory: {self.coverage_root}")

        job_to_pkg_tests: dict[str, dict[str, list[str]]] = {}

        for entry in sorted(root.iterdir()):
            if not entry.is_dir():
                continue

            suite_folder = entry
            metadata = self._read_metadata(suite_folder)
            job_name = metadata.get("job_name", suite_folder.name)
            test_name = metadata.get("test", suite_folder.name)

            coverage_dir = suite_folder / "coverage"
            if not coverage_dir.exists() or not coverage_dir.is_dir():
                print(color_message(f"No coverage/ folder in {suite_folder}", Color.ORANGE))
                continue

            coverage_txt = suite_folder / "coverage.txt"
            # Convert to textfmt using go tool covdata
            ctx.run(
                f"go tool covdata textfmt -i={coverage_dir} -o={coverage_txt}",
                echo=False,
                warn=True,
            )

            if not coverage_txt.exists():
                print(color_message(f"Failed to generate {coverage_txt}", Color.ORANGE))
                continue

            covered_packages = self._parse_coverage_file(coverage_txt)
            if not covered_packages:
                continue

            job_entry = job_to_pkg_tests.setdefault(job_name, {})
            for pkg in covered_packages:
                tests = job_entry.setdefault(pkg, [])
                if test_name not in tests:
                    tests.append(test_name)

        index = DynamicTestIndex(job_to_pkg_tests)
        return index

    def _read_metadata(self, suite_folder: Path) -> dict[str, str]:
        metadata_path = suite_folder / "metadata.json"
        if not metadata_path.exists():
            return {}
        try:
            with open(metadata_path, encoding="utf-8") as f:
                return json.load(f)
        except Exception as e:
            print(color_message(f"Error reading {metadata_path}: {e}", Color.ORANGE))
            return {}

    def _parse_coverage_file(self, coverage_txt: Path) -> set[str]:
        """Parse coverage text file and return set of covered package paths.

        Coverage lines example:
        github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe.go:24.13,25.2 2 1
        We consider a file covered when the last integer on the line (covered count) is > 0.
        A package is considered covered if any file in it is covered at least once.
        """
        covered: set[str] = set()
        try:
            with open(coverage_txt, encoding="utf-8") as f:
                for line in f:
                    if not line or line.startswith("mode:"):
                        continue
                    # Split at ':' first time to separate file path from the rest
                    parts = line.strip().split(":", 1)
                    if len(parts) < 2:
                        continue
                    file_path, rest = parts[0], parts[1]

                    # Determine if the line indicates any coverage > 0
                    # The rest contains positions and two integers, e.g. "24.13,25.2 2 1"
                    try:
                        tail_numbers = rest.strip().split()
                        if not tail_numbers:
                            continue
                        covered_count = int(tail_numbers[-1])
                        if covered_count <= 0:
                            continue
                    except Exception:
                        # If parsing fails, skip the line
                        continue

                    # Extract package path from full module path
                    # Remove .go suffix if present and take directory components after module root
                    if file_path.endswith(".go"):
                        file_path = file_path[:-3]
                    segments = file_path.split("/")
                    # Expect at least github.com DataDog datadog-agent.
                    if len(segments) < 3:
                        continue
                    # Build package path starting from after github.com/DataDog/datadog-agent, i.e., join from index 3
                    package_path = "/".join(segments[3:])
                    # Convert to package directory (drop filename)
                    if "/" in package_path:
                        package_path = "/".join(package_path.split("/")[:-1])
                    covered.add(package_path)
        except Exception as e:
            print(color_message(f"Error parsing coverage file {coverage_txt}: {e}", Color.ORANGE))
        return covered
