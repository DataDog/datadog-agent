"""
Agent release metrics collection scripts
"""

from datetime import datetime

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.utils import (
    GITHUB_REPO_NAME,
)

QUERY_URLS = {
    "before_freeze": "https://api.github.com/search/issues?q=repo:datadog/datadog-agent+type:pr+milestone:{}+merged:<{}",
    "on_freeze": "https://api.github.com/search/issues?q=repo:datadog/datadog-agent+type:pr+milestone:{}+merged:{}",
    "after_freeze": "https://api.github.com/search/issues?q=repo:datadog/datadog-agent+type:pr+milestone:{}+merged:>{}",
}


def get_release_lead_time(freeze_date, release_date):
    release_date = datetime.strptime(release_date, "%Y-%m-%d")
    freeze_date = datetime.strptime(freeze_date, "%Y-%m-%d")

    return (release_date - freeze_date).days


def get_prs_metrics(milestone, freeze_date):
    github = GithubAPI(repository=GITHUB_REPO_NAME)

    pr_counts = {"total": 0}

    for key, url in QUERY_URLS.items():
        _, prs = github.raw_request(url.format(milestone, freeze_date))
        pr_counts[key] = prs["total_count"]
        pr_counts["total"] += pr_counts[key]

    return pr_counts
