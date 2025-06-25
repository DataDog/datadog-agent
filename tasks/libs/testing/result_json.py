from dataclasses import dataclass
from datetime import datetime
from enum import Enum


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
        return cls(
            time=datetime.fromisoformat(data["Time"]),
            action=ActionType(data["Action"]),
            package=data["Package"],
            test=data.get("Test"),
            output=data.get("Output"),
        )


def run_is_failing(lines: list[ResultJsonLine]) -> bool:
    """Determines the failure status of the test run based on the actions in the lines."""
    is_fail = False
    for line in lines:
        if line.action == ActionType.FAIL:
            is_fail = True
        elif line.action == ActionType.PASS:
            # As soon as we find a PASS action, we can conclude the test run is not a failure (retry succeeded)
            is_fail = False
            break
    return is_fail
