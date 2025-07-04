from invoke.context import Context

from tasks.libs.common.status import Status


class RequirementState:
    def __init__(self, state: Status, reason: str, fixable: bool = False):
        self.state = state  # Should be one of Status.OK, Status.WARN, Status.FAIL
        self.reason = reason
        self.fixable = fixable

    def __repr__(self) -> str:
        return f"RequirementState(state={self.state}, reason='{self.reason}')"

    def __str__(self) -> str:
        msg = f"[{self.state}] {self.reason}"
        if self.fixable:
            msg += " (fixable)"
        return msg


class Requirement:
    name: str
    dependencies: list[type["Requirement"]] | None = None

    def check(self, _: Context, __: bool) -> list[RequirementState] | RequirementState:
        raise NotImplementedError
