import os
import sys

from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name, get_build_flags
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_DIR = os.path.join(".", "bin", "privateactionrunner")
BIN_PATH = os.path.join(BIN_DIR, bin_name("privateactionrunner"))


@task
@run_on_devcontainer
def build(
    ctx,
    install_path=None,
    flavor=AgentFlavor.base.name,
    rebuild=False,
    go_mod="readonly",
):
    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path)

    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/privateactionrunner/windows_resources/privateactionrunner.rc",
            vars=vars,
            out="cmd/privateactionrunner/rsrc.syso",
        )

    build_tags = get_default_build_tags(build="privateactionrunner", flavor=AgentFlavor[flavor])
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/privateactionrunner",
        build_tags=build_tags,
        ldflags=ldflags,
        gcflags=gcflags,
        rebuild=rebuild,
        env=env,
        bin_path=BIN_PATH,
        mod=go_mod,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
    )
