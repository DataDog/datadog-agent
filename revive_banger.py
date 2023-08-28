import subprocess
from codeowners import CodeOwners
import os
import sys

def create_pr_text(team_name: str) -> str:
    return f"""
### What does this PR do?

This PR fixes the `revive` linter error for the {team_name} team's owned files.

### Motivation

The `revive` linter was disabled for some time, which made us miss some linter errors. We'll be re-enabling `revive` on August 21st 2023

### Additional Notes

### Possible Drawbacks / Trade-offs

### Describe how to test/QA your changes

### Reviewer's Checklist


- [ ] If known, an appropriate milestone has been selected; otherwise the `Triage` milestone is set.
- [ ] Use the `major_change` label if your change either has a major impact on the code base, is impacting multiple teams or is changing important well-established internals of the Agent. This label will be use during QA to make sure each team pay extra attention to the changed behavior. For any customer facing change use a releasenote.
- [ ] A [release note](https://github.com/DataDog/datadog-agent/blob/main/docs/dev/contributing.md#reno) has been added or the `changelog/no-changelog` label has been applied.
- [ ] Changed code has automated tests for its functionality.
- [ ] Adequate QA/testing plan information is provided if the `qa/skip-qa` label is not applied.
- [ ] At least one `team/..` label has been applied, indicating the team(s) that should QA this change.
- [ ] If applicable, docs team has been notified or [an issue has been opened on the documentation repo](https://github.com/DataDog/documentation/issues/new).
- [ ] If applicable, the `need-change/operator` and `need-change/helm` labels have been applied.
- [ ] If applicable, the `k8s/<min-version>` label, indicating the lowest Kubernetes version compatible with this feature.
- [ ] If applicable, the [config template](https://github.com/DataDog/datadog-agent/blob/main/pkg/config/config_template.yaml) has been updated.

"""

# List all commits
# git cherry -v main
# + 84a10c0fb15819f4f6b57984504f0a5b414c3a8f Nik1

parent_commit_of_branch = "a0cde536dff38064e5d801636294330fc8a4bc06"

git_output = """+ f2fe899878727d4bed610e963b1106d260a0c856 Remove blank-imports warnings from revive
+ a9ba4db9fae6fbcf5eee250af57c29f680dd7ca3 Remove error-naming warnings from revive
+ 10ccb1fedfbd171c0b868b33fd9e1fc67e70e848 Remove error-strings warnings from revive
+ 5570fce52206ab110b836ac53ba17fea92c5d882 Remove increment-decrement warnings from revive
+ 0a548fa99a0d53d32e2d9cc76ee2f8bcd312d99c Remove indent-error-flow warnings from revive
+ 45b09103e12ebd1625f60b1d41db673d6a9abd00 Remove receiver-naming warnings from revive
+ 767d14bc4a9117885dfffd67b822ad58e1230f4a Remove unexported-return warnings from revive
+ 8012e9064c2085235b13d062556760e56163f204 Remove var-declaration warnings from revive
+ bcb1cc57f90d8a217c95f160a3096c73045abf57 Remove package-comments warnings from revive
+ 506c781fd9430a7070e016df1af98cc50b779d07 Remove exported comments from revive
+ 53d2b362a8a10a25f6bf51a7851edeed6bcf1b38 Remove exported comments rename from revive
+ 7b23500b5c4d35e87cb90c56550b7cf76abf1ed4 Remove exported naming from revive
+ 7c829fb7dbc384c329403a2f5ee7a74c1c78e896 Remove last exported warnings from revive
+ bc64728dd515b111ff6c87c0ce067206d77cead9 Remove last package-comments warnings from revive
+ f348e322027f7bfd9f464583a012832dbfacfc32 Remove var-naming warnings from revive
+ 077f2ea5027775800f133e6c4c1b0148e953a3a3 Activate revive
+ ea5a6767678aa6704d3b96578defd0df414fb661 Format files
+ aaaaaaaaaaaaaaaaaaaaaaaaaamerge_commit_1 Nik1
+ a280c55e8450ec0d85bb2e06ceb79d5c92beb41a Fix revive comments after merge
+ e9ebbeb14d1e5edfee26defdba6f4441a06bcec3 Code review
+ 0a234e378bdacc69a1ed2bd510d3b6dd9aceee09 fix copyright and component linters
+ 59d10b57a2ce2542ab9515678f935e265b9537ab Fix revive issues raised by ci
+ 9da0321a444a52ad9b445c50196699e781811cb0 Fix revive exported comments
+ 3f9e81d6ed3fd189b88a67a1e8642de9df163f76 Add comments for revive exported stuff
+ bbd36340bbbc0d118f40e45c44466443bfef77da Rename exported for revive
+ 4d5518bc891c8140aff0853d5abbce05080e36f1 More package-comments revive
+ e8861c4c058b7356d69438ac9564e3a6bc85307d Last revive comments
+ 796ff3a01288671d9c0ae631ca1e0d960098eaf6 Remove new var-naming revive comments
+ bd332812efa7074db951fc7a856013a1e824f908 Formatting
+ dd39a2a3374c0e986f60b2420523f25b83b32b9c more revive fix
+ c3e88ae5b36d7fbd5487e6940737a963a6131d84 new revive fix
+ aaaaaaaaaaaaaaaaaaaaaaaaaamerge_commit_2 Nik2"""

commits_list = [c[2:42] for c in git_output.split('\n')]

# List all modified files in a commit
# git diff-tree --no-commit-id --name-only bd61ad98 -r

with open(".github/CODEOWNERS", 'r') as f:
    codeowners = CodeOwners(f.read())

files_list = ""
for commit in commits_list:
    proc = subprocess.Popen(["git", "diff-tree", "--no-commit-id", "--name-only", commit, "-r"], stdout=subprocess.PIPE)
    files_list += proc.stdout.read().decode('utf-8')
files_list = files_list.split('\n')

list_files_per_team = dict()
leftover_files_to_sort = []

for f in files_list:
    if codeowners.of(f) == []:
        leftover_files_to_sort.append(f)
    else:
        if codeowners.of(f)[0][1] not in list_files_per_team:
            list_files_per_team[codeowners.of(f)[0][1]] = []
        list_files_per_team[codeowners.of(f)[0][1]].append(f)

# print(list_files_per_team)
# print(leftover_files_to_sort)


# Cherry-pick all 30 commits

for commit in commits_list:
    if commit == "aaaaaaaaaaaaaaaaaaaaaaaaaamerge_commit_1":
        print("git cherry-pick -n 84a10c0fb15819f4f6b57984504f0a5b414c3a8f -m ea5a6767678aa6704d3b96578defd0df414fb661")
        subprocess.call(["git", "cherry-pick", "-n", "84a10c0fb15819f4f6b57984504f0a5b414c3a8f", "-m", "1"])
        continue

    if commit == "aaaaaaaaaaaaaaaaaaaaaaaaaamerge_commit_2":
        print("git cherry-pick -n 202ceed8b7c1259910a90ade3e480e1105330abf -m ea5a6767678aa6704d3b96578defd0df414fb661")
        subprocess.call(["git", "cherry-pick", "-n", "202ceed8b7c1259910a90ade3e480e1105330abf", "-m", "1"])
        continue

    print(f"git cherry-pick -n {commit}")
    subprocess.call(["git", "cherry-pick", "-n", commit])

# Cherry-pick merge commits

# For each team_name
#       git checkout on main
#       create a branch named f"revive/{team_name}"
#       git add all files in list_files_per_team[team_name]
#       git commit -m f"Revive linter fixes for {team_name}"
#       git push --set-upstream origin f"revive/{team_name}"

for team_name, team_files in list_files_per_team.items():
    print(f"Starting branch for team {team_name}")
    print("git checkout main")
    subprocess.call(["git", "checkout", parent_commit_of_branch])
    print(f"git checkout -b revive/{team_name}")
    subprocess.call(["git", "checkout", "-b", f"revive/{team_name}"])
    for file in team_files:
        print(f"git add {file}")
        subprocess.call(["git", "add", file])
    print(f"git commit -m" 'Revive linter fixes for {team_name}')
    subprocess.call(["git", "commit", "-m", f"'Revive linter fixes for {team_name}'"])
    print(f"git push --set-upstream origin revive/{team_name}")
    subprocess.call(["git", "push", "--set-upstream", "origin", f"revive/{team_name}"])
    print(f"Branch pushed for team {team_name}. Go to https://github.com/DataDog/datadog-agent to create the PR.")
    subprocess.check_call(["read", "-s", "-p", "Press Enter to continue..."])
    print(f"PR text for team in your clipboard")
    pr_text = create_pr_text(team_name)
    subprocess.run("pbcopy", text=True, input=pr_text)

