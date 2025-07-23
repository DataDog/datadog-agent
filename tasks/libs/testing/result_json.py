import json
from collections import defaultdict
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from functools import cached_property


class ActionType(Enum):
    START = "start"  # noqa: F841
    RUN = "run"  # noqa: F841
    OUTPUT = "output"  # noqa: F841
    FAIL = "fail"
    PASS = "pass"
    SKIP = "skip"  # noqa: F841
    PAUSE = "pause"  # noqa: F841
    CONT = "cont"  # noqa: F841


@dataclass
class ResultJsonLine:
    time: datetime
    action: ActionType
    package: str

    # The test name is optional, as some lines may correspond to package-level actions
    test: str | None = None
    # The output is optional, as some lines may not have any output associated with them
    output: str | None = None

    @classmethod
    def from_dict(cls, data: dict) -> "ResultJsonLine":
        try:
            return cls(
                time=datetime.fromisoformat(data["Time"]),
                action=ActionType(data["Action"]),
                package=data["Package"],
                test=data.get("Test"),
                output=data.get("Output"),
            )
        except (KeyError, ValueError) as e:
            raise ValueError(f"Invalid data for ResultJsonLine: {data}") from e


@dataclass
class ResultJson:
    lines: list[ResultJsonLine]
    _package_tests_dict: dict[str, dict[str, list[ResultJsonLine]]] = field(
        init=False, repr=False, default_factory=dict
    )

    def __post_init__(self):
        self.lines.sort(key=lambda x: x.time)

    @classmethod
    def from_file(cls, file: str) -> "ResultJson":
        """Load a ResultJson from a file."""
        res = []
        with open(file) as f:
            for line in f:
                data = json.loads(line)
                try:
                    res.append(ResultJsonLine.from_dict(data))
                except ValueError:
                    # TODO(@agent-devx): Use a proper logging mechanism instead of print
                    print(f"WARNING: Invalid line in result json file, skipping: {line.strip()}")
        return cls(res)

    def _sort_into_packages_and_tests(self) -> dict[str, dict[str, list[ResultJsonLine]]]:
        """Sorts the parsed result lines into packages and tests."""
        result: dict[str, dict[str, list[ResultJsonLine]]] = defaultdict(lambda: defaultdict(list))

        for line in self.lines:
            package_dict = result[line.package]
            if not line.test:
                # Package-level action
                package_dict["_"].append(line)
            else:
                # Test-level action
                package_dict[line.test].append(line)

        return result

    @property
    def package_tests_dict(self) -> dict[str, dict[str, list[ResultJsonLine]]]:
        """Returns a dictionary of all ResultJsonLines sorted by package and test."""
        if not self._package_tests_dict:
            self._package_tests_dict = self._sort_into_packages_and_tests()

        return self._package_tests_dict

    @property
    def packages(self) -> set[str]:
        """Returns a set of all packages in the result."""
        return set(self.package_tests_dict.keys())

    @cached_property
    def failing_packages(self) -> set[str]:
        """Returns a set of packages which have package-level failures."""
        return {pkg for pkg, tests in self.package_tests_dict.items() if run_is_failing(tests.get("_", []))}

    @cached_property
    def failing_tests(self) -> dict[str, set[str]]:
        """Returns a dictionary of packages and their failing tests."""
        failing_tests: dict[str, set[str]] = defaultdict(set)

        for package, tests in self.package_tests_dict.items():
            for test_name, actions in tests.items():
                if test_name == "_":
                    # Skip package-level actions, as they are not tests
                    continue
                if run_is_failing(actions):
                    failing_tests[package].add(test_name)

        return failing_tests


def run_is_failing(lines: list[ResultJsonLine]) -> bool:
    """
    Determines the failure status of the test run based on the actions in the lines.
    A run is considered failing if it contains a FAIL action or if it has an output with "panic:" in it.
    Make sure the lines in `lines` all refer to the same test !
    """
    is_fail = False
    for line in lines:
        # Some test lines don't set their action to fail when they panic, but we should also consider that a failure
        if line.action == ActionType.FAIL or line.output and "panic:" in line.output:
            is_fail = True
        elif line.action == ActionType.PASS:
            # As soon as we find a PASS action, we can conclude the test run is not a failure (retry succeeded)
            is_fail = False
            break
    return is_fail


def merge_result_jsons(result_jsons: list[ResultJson]) -> ResultJson:
    """Merges multiple ResultJson objects into one."""
    merged_lines = []
    merged_results_dict: dict[str, dict[str, list[ResultJsonLine]]] = defaultdict(lambda: defaultdict(list))
    for result_json in result_jsons:
        merged_lines.extend(result_json.lines)
        for package, tests in result_json.package_tests_dict.items():
            for test, actions in tests.items():
                merged_results_dict[package][test].extend(actions)

    result = ResultJson(merged_lines)
    result._package_tests_dict = merged_results_dict  # pylint:disable=protected-access
    return result
