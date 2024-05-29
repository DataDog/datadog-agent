"""
Agent release metrics collection scripts
"""

from datetime import datetime

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.utils import (
    GITHUB_REPO_NAME,
)


def get_release_lead_time(freeze_date, release_date):
    release_date = datetime.strptime(release_date, "%Y-%m-%d")
    freeze_date = datetime.strptime(freeze_date, "%Y-%m-%d")

    return (release_date - freeze_date).days


def get_prs_metrics(milestone, freeze_date):
    github = GithubAPI(repository=GITHUB_REPO_NAME)
    freeze_date = datetime.strptime(freeze_date, "%Y-%m-%d").date()
    pr_counts = {"total": 0, "before_freeze": 0, "on_freeze": 0, "after_freeze": 0}
    m = get_milestone(github.repo, milestone)
    issues = github.repo.get_issues(m, state='closed')
    for issue in issues:
        if issue.pull_request is None or issue.pull_request.raw_data['merged_at'] is None:
            continue
        # until 3.11 we need to strip the date string
        merged = datetime.fromisoformat(issue.pull_request.raw_data['merged_at'][:-1]).date()
        if merged < freeze_date:
            pr_counts["before_freeze"] += 1
        elif merged == freeze_date:
            pr_counts["on_freeze"] += 1
        else:
            pr_counts["after_freeze"] += 1
    pr_counts["total"] = pr_counts["before_freeze"] + pr_counts["on_freeze"] + pr_counts["after_freeze"]
    return pr_counts


def get_milestone(repo, milestone):
    milestones = repo.get_milestones(state="all")
    for mile in milestones:
        if mile.title == milestone:
            return mile
    return None
