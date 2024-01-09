import json
import os
import re
from collections import Counter

from invoke import task

from tasks.libs.pipeline_notifications import read_owners


@task
def add_team_labels(ctx, pr_id, pr_title=None):
    """
    Add team labels to a PR based on the changed dependencies
    """

    codeowners = read_owners(".github/CODEOWNERS")

    # Get what was changed according to PR title
    if pr_title is None:
        title_words = ctx.run(f"gh pr view {pr_id} | head -n 1", hide=True).stdout.split()[1:]
    else:
        title_words = pr_title.split()
    dependency = title_words[1]
    if "group" in title_words:  # dependabot defines group. Currently dep name is part of the group name
        group_index = title_words.index("group")
        dependency = title_words[group_index - 1]
    import_module = re.compile(rf"^[ \t]*\"{dependency}.*$")
    folder = {title_words[-1][1:]} if title_words[-2] == "in" else "."

    # Find the responsible person for each file
    owners = []
    for root, _, files in os.walk(folder):
        if root == "./.git" or any(root.startswith(prefix) for prefix in ["./venv", "./.git/"]):
            continue
        for file in files:
            file_path = os.path.join(root, file)
            norm_path = os.path.normpath(file_path)
            if "docs/" in file_path.casefold():
                continue
            with open(file_path, "r") as f:
                try:
                    for line in f:
                        if is_go_module(dependency):
                            if import_module.match(line):
                                owners.extend([owner[1] for owner in codeowners.of(norm_path)])
                                break
                        elif dependency in line:
                            owners.extend([owner[1] for owner in codeowners.of(norm_path)])
                            break
                except UnicodeDecodeError:
                    continue
    c = Counter(owners)
    responsible = c.most_common(1)[0][0].replace('@Datadog/', '').replace('@DataDog/', '')
    # Hardcode for USM as owner name does not match team label name
    if responsible == "universal-service-monitoring":
        responsible = "usm"

    # Retrieve & assign labels
    team_labels = [team["name"] for team in json.loads(ctx.run("gh label list -S team --json name", hide=True).stdout)]
    try:
        label = next(label for label in team_labels if responsible in label)
    except StopIteration:
        label = "team/agent-platform"
    ctx.run(f"gh pr edit {pr_id} --remove-label \"team/triage\"")
    ctx.run(f"gh pr edit {pr_id} --add-label \"{label}\"")


def is_go_module(module):
    starts = [
        "cloud.google.com",
        "code.cloudfoundry.org",
        "contrib.go.opencensus.io",
        "dario.cat",
        "github.com",
        "go.etcd.io",
        "go.mongodb.org",
        "go.opencensus.io",
        "go.opentelemetry.io",
        "go.uber.org",
        "go4.org",
        "golang.org",
        "gomodules.xyz",
        "gonum.org",
        "google.golang.org",
        "gopkg.in",
        "gotest.tools",
        "honnef.co",
        "inet.af",
        "k8s.io",
        "lukechampine.com",
        "mellium.im",
        "modernc.org",
        "oras.land",
        "sigs.k8s.io",
    ]
    return any(module.startswith(start) for start in starts)
