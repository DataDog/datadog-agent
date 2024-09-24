from __future__ import annotations

import os
import sys
from datetime import timedelta

from invoke import Context, task
from invoke.exceptions import Exit

import tasks.libs.notify.unit_tests as unit_tests_utils
from tasks.github_tasks import pr_commenter
from tasks.libs.ciproviders.gitlab_api import (
    MultiGitlabCIDiff,
    get_all_gitlab_ci_configurations,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import DEFAULT_BRANCH
from tasks.libs.common.datadog_api import send_metrics
from tasks.libs.common.utils import gitlab_section
from tasks.libs.notify import alerts, failure_summary, pipeline_status
from tasks.libs.notify.utils import PROJECT_NAME
from tasks.libs.pipeline.notifications import (
    check_for_missing_owners_slack_and_jira,
)
from tasks.libs.pipeline.stats import compute_failed_jobs_series, compute_required_jobs_max_duration


@task
def check_teams(_):
    if check_for_missing_owners_slack_and_jira():
        print(
            "Error: Some teams in CODEOWNERS don't have their slack notification channel or jira specified!\n"
            "Please specify one in the GITHUB_SLACK_MAP or GITHUB_JIRA_MAP maps in tasks/libs/pipeline/github_slack_map.yaml"
            " or tasks/libs/pipeline/github_jira_map.yaml"
        )
        raise Exit(code=1)
    else:
        print("All CODEOWNERS teams have their slack notification channel and jira project specified !!")


@task
def send_message(ctx: Context, notification_type: str = "merge", dry_run: bool = False):
    """
    Send notifications for the current pipeline. CI-only task.
    Use the --dry-run option to test this locally, without sending
    real slack messages.
    """
    pipeline_status.send_message(ctx, notification_type, dry_run)


@task
def send_stats(_, dry_run=False):
    """
    Send statistics to Datadog for the current pipeline. CI-only task.
    Use the --dry-run option to test this locally, without sending
    data points to Datadog.
    """
    if not (dry_run or os.environ.get("DD_API_KEY")):
        print("DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend")
        raise Exit(code=1)

    series = compute_failed_jobs_series(PROJECT_NAME)
    series.extend(compute_required_jobs_max_duration(PROJECT_NAME))

    if not dry_run:
        send_metrics(series)
        print(f"Sent pipeline metrics: {series}")
    else:
        print(f"Would send: {series}")


@task
def check_consistent_failures(ctx, job_failures_file="job_executions.v2.json"):
    # Retrieve the stored document in aws s3. It has the following format:
    # {
    #     "pipeline_id": 123,
    #     "jobs": {
    #         "job1": {"consecutive_failures": 2, "jobs_info": [{"id": null, "failing": false, "commit": "abcdef42"}, {"id": 314618, "failing": true, "commit": "abcdef42"}, {"id": 618314, "failing": true, "commit": "abcdef42"}]},
    #         "job2": {"consecutive_failures": 0, "cumulative_failures": [{"id": 314618, "failing": true, "commit": "abcdef42"}, {"id": null, "failing": false, "commit": "abcdef42"}]},
    #         "job3": {"consecutive_failures": 1, "cumulative_failures": [{"id": 314618, "failing": true, "commit": "abcdef42"}]},
    #     }
    # }
    # NOTE: this format is described by the Executions class
    # The pipeline_id is used to by-pass the check if the pipeline chronological order is not respected
    # The jobs dictionary contains the consecutive and cumulative failures for each job
    # The consecutive failures are reset to 0 when the job is not failing, and are raising an alert when reaching the CONSECUTIVE_THRESHOLD (3)
    # The cumulative failures list contains 1 for failures, 0 for succes. They contain only then CUMULATIVE_LENGTH(10) last executions and raise alert when 50% failure rate is reached

    job_executions = alerts.retrieve_job_executions(ctx, job_failures_file)

    # By-pass if the pipeline chronological order is not respected
    if job_executions.pipeline_id > int(os.environ["CI_PIPELINE_ID"]):
        return
    job_executions.pipeline_id = int(os.environ["CI_PIPELINE_ID"])

    alert_jobs, job_executions = alerts.update_statistics(job_executions)

    alerts.send_notification(ctx, alert_jobs)

    alerts.upload_job_executions(ctx, job_executions, job_failures_file)


@task
def failure_summary_upload_pipeline_data(ctx):
    """
    Upload failure summary data to S3 at the end of each main pipeline
    """
    failure_summary.upload_summary(ctx, os.environ['CI_PIPELINE_ID'])


@task
def failure_summary_send_notifications(
    ctx, daily_summary: bool = False, weekly_summary: bool = False, max_length: int = 8, dry_run: bool = False
):
    """
    Make summaries from data in s3 and send them to slack
    """

    assert (
        daily_summary or weekly_summary and not (daily_summary and weekly_summary)
    ), "Exactly one of daily or weekly summary must be set"

    period = timedelta(days=1) if daily_summary else timedelta(weeks=1)
    failure_summary.send_summary_messages(ctx, weekly_summary, max_length, period, dry_run=dry_run)


@task
def unit_tests(ctx, pipeline_id, pipeline_url, branch_name, dry_run=False):
    jobs_with_no_tests_run = unit_tests_utils.process_unit_tests_tarballs(ctx)
    msg = unit_tests_utils.create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)

    if dry_run:
        print(msg)
    else:
        unit_tests_utils.comment_pr(msg, pipeline_id, branch_name, jobs_with_no_tests_run)


@task
def gitlab_ci_diff(ctx, before: str | None = None, after: str | None = None, pr_comment: bool = False):
    """
    Creates a diff from two gitlab-ci configurations.

    - before: Git ref without new changes, None for default branch
    - after: Git ref with new changes, None for current local configuration
    - pr_comment: If True, post the diff as a comment in the PR
    - NOTE: This requires a full api token access level to the repository
    """

    from tasks.libs.ciproviders.github_api import GithubAPI

    pr_comment_head = 'Gitlab CI Configuration Changes'
    if pr_comment:
        github = GithubAPI()

        if (
            "CI_COMMIT_BRANCH" not in os.environ
            or len(list(github.get_pr_for_branch(os.environ["CI_COMMIT_BRANCH"]))) != 1
        ):
            print(
                color_message("Warning: No PR found for current branch, skipping message", Color.ORANGE),
                file=sys.stderr,
            )
            pr_comment = False

    if pr_comment:
        job_url = os.environ['CI_JOB_URL']

    try:
        before_name = before or "merge base"
        after_name = after or "local files"

        # The before commit is the LCA commit between before and after
        before = before or DEFAULT_BRANCH
        before = ctx.run(f'git merge-base {before} {after or "HEAD"}', hide=True).stdout.strip()

        print(f'Getting after changes config ({color_message(after_name, Color.BOLD)})')
        after_config = get_all_gitlab_ci_configurations(ctx, git_ref=after, clean_configs=True)

        print(f'Getting before changes config ({color_message(before_name, Color.BOLD)})')
        before_config = get_all_gitlab_ci_configurations(ctx, git_ref=before, clean_configs=True)

        diff = MultiGitlabCIDiff(before_config, after_config)

        if not diff:
            print(color_message("No changes in the gitlab-ci configuration", Color.GREEN))

            # Remove comment if no changes
            if pr_comment:
                pr_commenter(ctx, pr_comment_head, delete=True, force_delete=True)

            return

        # TODO: test
        comment_summary = diff.display(cli=False, job_url=job_url, only_summary=True)

        # Display diff
        print('\nGitlab CI configuration diff:')
        with gitlab_section('Gitlab CI configuration diff'):
            print(diff.display(cli=True))

        if pr_comment:
            print('\nSending / updating PR comment')
            comment = diff.display(cli=False, job_url=job_url)
            try:
                pr_commenter(ctx, pr_comment_head, comment)
            except Exception:
                # Comment too large
                print(color_message('Warning: Failed to send full diff, sending only changes summary', Color.ORANGE))

                comment_summary = diff.display(cli=False, job_url=job_url, only_summary=True)
                try:
                    pr_commenter(ctx, pr_comment_head, comment_summary)
                except Exception:
                    print(color_message('Warning: Failed to send summary diff, sending only job link', Color.ORANGE))

                    pr_commenter(
                        ctx,
                        pr_comment_head,
                        f'Cannot send only summary message, see the [job log]({job_url}) for details',
                    )

            print(color_message('Sent / updated PR comment', Color.GREEN))
    except Exception:
        if pr_comment:
            # Send message
            pr_commenter(
                ctx,
                pr_comment_head,
                f':warning: *Failed to display Gitlab CI configuration changes, see the [job log]({job_url}) for details.*',
            )

        raise
