import os
import subprocess
import sys

from tasks.libs.common.color import color_message


def handle_go_work():
    """
    If go workspaces aren't explicitly enabled from the environment but there is a go.work file,
    this function will print a warning and export GOWORK=off so that subprocesses don't use workspaces.

    At least the following go commands behave differently with workspaces:
    - go build
    - go list
    - go run
    - go test
    - go vet
    - go work
    """
    if "GOWORK" in os.environ:
        # go work is explicitly set
        return

    try:
        # find the go.work file
        # according to the blog https://go.dev/blog/get-familiar-with-workspaces :
        # "The output is empty if the go command is not in workspace mode."
        # meaning it didn't find a go.work file in the ancestor directories and GOWORK is not set
        res = subprocess.run(["go", "env", "GOWORK"], capture_output=True)
        if res.returncode != 0 or res.stdout.decode('UTF-8').strip() == "":
            return
    except Exception:
        # go command not found, no need to care about workspaces
        return

    print(
        color_message(
            "Disabling GOWORK to avoid failures or weird behavior.\n"
            "It can be enabled by setting the GOWORK environment variable to the empty string, "
            "or to the absolute path of a go.work file.",
            "orange",
        ),
        file=sys.stderr,
    )
    os.environ["GOWORK"] = "off"
