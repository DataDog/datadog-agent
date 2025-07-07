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
        msg = f"[{self.state}]"

        if self.state != Status.OK and self.reason:
            msg += f" {self.reason}"

        if self.fixable:
            msg += " (fixable)"
        return msg


class Requirement:
    dependencies: list[type["Requirement"]] | None = None

    def check(self, _: Context, __: bool) -> list[RequirementState] | RequirementState:
        raise NotImplementedError


def summarize_requirement_states(states: list[RequirementState] | RequirementState) -> RequirementState:
    if isinstance(states, RequirementState):
        return states

    final_state = Status.OK
    reasons: list[str] = []
    fixable: list[bool] = []

    for state in states:
        if final_state == Status.OK:
            final_state = state.state
        elif final_state == Status.WARN and state.state == Status.FAIL:
            final_state = Status.FAIL

        if state.state != Status.OK:
            fixable.append(state.fixable)
            reasons.append(state.reason)

    final_reason = "\n".join(reasons) if reasons else "all requirements are met"

    return RequirementState(final_state, final_reason, len(fixable) > 0 and all(fixable))
