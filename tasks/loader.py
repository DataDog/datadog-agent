from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build


@task
@run_on_devcontainer
def build(
    ctx,
):
    build_tags = get_default_build_tags(build="loader", flavor=AgentFlavor.base)
    go_build(ctx, f"{REPO_PATH}/cmd/loader", build_tags=build_tags)
