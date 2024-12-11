"""
Emacs namespaced tags

Helpers for getting Emacs set up nicely
"""

from invoke import task

from tasks.build_tags import build_tags, compute_config_build_tags
from tasks.flavor import AgentFlavor


@task(
    help={
        "targets": f"Comma separated list of targets to include. Possible values: all, {', '.join(build_tags[AgentFlavor.base].keys())}. Default: all",
        "flavor": f"Agent flavor to use. Possible values: {', '.join(AgentFlavor.__members__.keys())}. Default: {AgentFlavor.base.name}",
    }
)
def set_buildtags(
    _,
    targets="all",
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
):
    """
    Create Emacs .dir-locals.el settings file for this project to include correct build tags
    """
    use_tags = compute_config_build_tags(
        targets=targets,
        build_include=build_include,
        build_exclude=build_exclude,
        flavor=flavor,
    )

    with open(".dir-locals.el", "w") as f:
        f.write(f'((go-mode . ((lsp-go-build-flags . ["-tags" "{",".join(sorted(use_tags))}"]))))\n')
