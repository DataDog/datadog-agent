import os
import shutil
import sys

from invoke import task

from tasks.build_tags import get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_NAME = "mini-agent"
BIN_DIR = os.path.join(".", "bin", "mini-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("mini-agent"))


@task
def build(ctx, flavor=AgentFlavor.base.name):
    """
    Build the mini-agent
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    flavor = AgentFlavor[flavor]
    env = {"GO111MODULE": "on"}
    build_tags = get_default_build_tags(build="agent", flavor=flavor)
    ldflags = get_version_ldflags(ctx)
    if os.environ.get("DELVE"):
        gcflags = "all=-N -l"
    else:
        gcflags = ""

    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/mini-agent/windows_resources/mini-agent.rc",
            vars=vars,
            out="cmd/mini-agent/rsrc.syso",
        )

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/mini-agent",
        mod="readonly",
        build_tags=build_tags,
        ldflags=ldflags,
        gcflags=gcflags,
        bin_path=BIN_PATH,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
        env=env,
    )

    dist_folder = os.path.join(BIN_DIR, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    # Copy sample config if it exists
    sample_config = "./cmd/mini-agent/dist/datadog.yaml"
    if os.path.exists(sample_config):
        shutil.copy(sample_config, os.path.join(dist_folder, "datadog.yaml"))
