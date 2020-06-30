"""
Boostrapping related logic goes here
"""
import os
import json
import sys

from .utils import get_gopath

# Bootstrap dependencies description
BOOTSTRAP_DEPS = "bootstrap.json"
BOOTSTRAP_ORDER_KEY = "order"
BOOTSTRAP_SUPPORTED_KINDS = ["go", "python", "shell"]
BOOTSTRAP_SUPPORTED_STEPS = ["checkout", "install"]


def get_deps(key):
    """
    Load dependency file, return specific key
    """
    here = os.path.abspath(os.path.dirname(__file__))
    with open(os.path.join(here, '..', BOOTSTRAP_DEPS)) as depfile:
        deps = json.load(depfile)

    return deps.get(key, {})


def process_deps(ctx, target, version, kind, step, cmd=None, verbose=False):
    """
    Process a dependency target.

    Keyword arguments:
    target -- the package name
    version -- the target version
    kind -- go, python
    step -- checkout, install
    verbose -- boolean
    """
    if kind not in BOOTSTRAP_SUPPORTED_KINDS:
        raise Exception("Unknown dependency kind: {} for {}".format(kind, target))

    if step not in BOOTSTRAP_SUPPORTED_STEPS:
        raise Exception("Unknown bootstrap step: {} for {}".format(step, target))

    verbosity = ' -v' if verbose else ''
    if kind == "go":
        if step == "checkout":
            # download tools
            path = os.path.join(get_gopath(ctx), 'src', target)
            if not os.path.exists(path):
                ctx.run("go get{} -d -u {}".format(verbosity, target), env={'GO111MODULE': 'off'})

            with ctx.cd(path):
                # checkout versions
                ctx.run("git fetch")
                ctx.run("git checkout {}".format(version))
        elif step == "install":
            ctx.run("go install{} {}".format(verbosity, target), env={'GO111MODULE': 'off'})
    elif kind == "python":
        # no checkout needed for python deps
        if step == "install":
            ctx.run("pip install{} {}=={}".format(verbosity, target, version))
    elif kind == "shell":
        if step == "install":
            if sys.platform == 'win32':
                print("shell dependencies currently unsupported on windows please install manually")
            else:
                ctx.run("{}".format(cmd), env={'BOOTSTRAPPED_VERSION': version})
