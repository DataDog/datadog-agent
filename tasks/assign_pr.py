import json
import os
import re

from invoke import task

from tasks.libs.owners.parsing import read_owners


class Owner:
    def __init__(self, name, github_team, label="team/agent-platform"):
        self.name = name
        self.github_team = github_team
        self.label = label

    def __hash__(self) -> int:
        return hash(self.name) ^ hash(self.github_team)

    def __eq__(self, __value) -> bool:
        return isinstance(__value, Owner) and self.name == __value.name and self.github_team == __value.github_team

    def __repr__(self) -> str:
        return f"Owner(name:{self.name}, github_team:{self.github_team}, label:{self.label})"


@task
def add_labels_and_reviewers(ctx, pr_id, pr_title=None):
    """
    Add team labels and reviewers to a dependabot bump PR based on the changed dependencies
    """

    codeowners = read_owners(".github/CODEOWNERS")

    # Get what was changed according to PR title
    if pr_title is None:
        title_words = ctx.run(f"gh pr view {pr_id} | head -n 1", hide=True).stdout.split()[1:]
    else:
        title_words = pr_title.split()
    if title_words[0] != "Bump":
        print("This is not a (dependabot) bump PR, this action should not be run on it.")
        return
    dependency = title_words[1]
    if "group" in title_words:  # dependabot defines group. Currently dep name is part of the group name
        group_index = title_words.index("group")
        dependency = title_words[group_index - 1]
    import_module = re.compile(rf"^[ \t]*\"{dependency}.*$")
    folder = {title_words[-1][1:]} if title_words[-2] == "in" else "."

    # Find the responsible person for each file
    owners = set()
    for root, _, files in os.walk(folder):
        if root == "./.git" or any(root.startswith(prefix) for prefix in ["./venv", "./.git/"]):
            continue
        for file in files:
            file_path = os.path.join(root, file)
            norm_path = os.path.normpath(file_path)
            if "docs/" in file_path.casefold():
                continue
            with open(file_path) as f:
                try:
                    for line in f:
                        if is_go_module(dependency):
                            if import_module.match(line):
                                owners.add(get_owner(norm_path, codeowners))
                                break
                        elif dependency in line:
                            owners.add(get_owner(norm_path, codeowners))
                            break
                except UnicodeDecodeError:
                    continue

    # Retrieve & assign labels and reviewers
    team_labels = [team["name"] for team in json.loads(ctx.run("gh label list -S team --json name", hide=True).stdout)]
    for owner in owners:
        try:
            owner.label = next(label for label in team_labels if owner.name in label)
        except StopIteration:
            pass  # Agent platform is already set by default
    ctx.run(f"gh pr edit {pr_id} --remove-label \"team/triage\"")
    ctx.run(f"gh pr edit {pr_id} --add-label \"{','.join(owner.label for owner in owners)}\"")
    ctx.run(f"gh pr edit {pr_id} --add-reviewer \"{','.join(owner.github_team for owner in owners)}\"")


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


def get_owner(file_path, codeowners):
    github_team = codeowners.of(file_path)[0][1]
    team_name = github_team.replace("@Datadog/", "").replace("@DataDog/", "")
    if team_name == "universal-service-monitoring":
        team_name = "usm"
    return Owner(team_name, github_team)
