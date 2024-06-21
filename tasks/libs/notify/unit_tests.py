from __future__ import annotations

import os
import re
import tempfile

from invoke import task


# TODO A
# if comment found:
#   send_comment(get_message(comment))
# else:
#   send_comment(get_message(None))
def pr_commenter(header: str, branch_name: str, message: str | callable[str | None, str], repo="DataDog/datadog-agent"):
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI(repo)
    prs = gh.get_pr_for_branch(branch_name)

    if prs.totalCount == 0:
        # TODO A : Warning
        # If the branch is not linked to any PR we stop here
        return
    pr = prs[0]

    comment = gh.find_comment(pr.number, header)
    if comment is None:
        msg = message if isinstance(message, str) else message(None, "")
        gh.publish_comment(pr.number, msg)
        return

    msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
    comment.edit(msg)


def pr_commenter(header: str, branch_name: str, repo="DataDog/datadog-agent"):
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI(repo)
    prs = gh.get_pr_for_branch(branch_name)

    if prs.totalCount == 0:
        # If the branch is not linked to any PR we stop here
        return
    pr = prs[0]

    comment = gh.find_comment(pr.number, header)
    if comment is None and len(jobs_with_no_tests_run) > 0:
        msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
        gh.publish_comment(pr.number, msg)
        return

    if comment is None:
        # If no tests are executed and no previous comment exists, we stop here
        return

    previous_comment_pipeline_id = pipeline_id_regex.findall(comment.body)
    # An older pipeline should not edit a message corresponding to a newer pipeline
    if previous_comment_pipeline_id and previous_comment_pipeline_id[0] > pipeline_id:
        return

    if len(jobs_with_no_tests_run) > 0:
        msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
        comment.edit(msg)
    else:
        comment.delete()


@task
def unit_tests(ctx, pipeline_id, pipeline_url, branch_name):
    pipeline_id_regex = re.compile(r"pipeline ([0-9]*)")

    jobs_with_no_tests_run = process_unit_tests_tarballs(ctx)
    pr_commenter(branch_name, "[Fast Unit Tests Report]")


def create_msg(pipeline_id, pipeline_url, job_list):
    # TODO A : Remove header
    msg = f"""
[Fast Unit Tests Report]

On pipeline [{pipeline_id}]({pipeline_url}) ([CI Visibility](https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Apipeline%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40ci.pipeline.id%3A{pipeline_id}&fromUser=false&index=cipipeline)). The following jobs did not run any unit tests:

<details>
<summary>Jobs:</summary>

"""
    for job in job_list:
        msg += f"  - {job}\n"
    msg += "</details>\n"
    msg += "\n"
    msg += "If you modified Go files and expected unit tests to run in these jobs, please double check the job logs. If you think tests should have been executed reach out to #agent-developer-experience"
    return msg


def process_unit_tests_tarballs(ctx):
    tarballs = ctx.run("ls junit-tests_*.tgz", hide=True).stdout.split()
    jobs_with_no_tests_run = []
    for tarball in tarballs:
        with tempfile.TemporaryDirectory() as unpack_dir:
            ctx.run(f"tar -xzf {tarball} -C {unpack_dir}")

            # We check if the folder contains at least one junit.xml file. Otherwise we consider no tests were executed
            if not any(f.endswith(".xml") for f in os.listdir(unpack_dir)):
                jobs_with_no_tests_run.append(
                    tarball.replace("junit-", "").replace(".tgz", "").replace("-repacked", "")
                )  # We remove -repacked to have a correct job name macos

    return jobs_with_no_tests_run
