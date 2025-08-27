import getpass

from invoke.context import Context

from tasks.kernel_matrix_testing.setup.requirement import Requirement, RequirementState
from tasks.libs.common.status import Status

from .common import Docker


def get_requirements() -> list[Requirement]:
    return [UserInDockerGroup()]


class UserInDockerGroup(Requirement):
    dependencies: list[type[Requirement]] = [Docker]

    def check(self, ctx: Context, _: bool) -> RequirementState:
        ret = ctx.run(
            "cat /proc/$$/status | grep '^Groups:' | grep $(cat /etc/group | grep 'docker:' | cut -d ':' -f 3)",
            warn=True,
        )
        if ret is None or not ret.ok:
            return RequirementState(
                Status.FAIL,
                f"User '{getpass.getuser()}' is not in docker group. Please resolve this https://docs.docker.com/engine/install/linux-postinstall/",
            )

        return RequirementState(Status.OK, "User is in docker group.")
