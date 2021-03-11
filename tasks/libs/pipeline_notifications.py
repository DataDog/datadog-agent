import os
import subprocess
from collections import defaultdict
from pprint import pprint

from .common.gitlab import Gitlab


def get_failed_jobs(project_name, pipeline_id):
    gitlab = Gitlab()

    jobs = gitlab.all_jobs(project_name, pipeline_id)

    # Get instances of failed jobs
    failed_jobs = {job["name"]: [] for job in jobs if job["status"] == "failed"}

    # Group jobs per name
    for job in jobs:
        if job["name"] in failed_jobs:
            failed_jobs[job["name"]].append(job)

    # There, we now have the following map:
    # job name -> list of jobs with that name, including at least one failed job

    final_failed_jobs = []
    for job_name, jobs in failed_jobs.items():
        # We sort each list per creation date
        jobs.sort(key=lambda x: x["created_at"])
        # Check the final job in the list: it contains the current status of the job
        final_status = {
            "name": job_name,
            "stage": jobs[-1]["stage"],
            "status": jobs[-1]["status"],
            "allow_failure": jobs[-1]["allow_failure"],
            "url": jobs[-1]["web_url"],
            "retry_summary": [job["status"] for job in jobs],
        }
        final_failed_jobs.append(final_status)

    pprint(final_failed_jobs)

    return final_failed_jobs


def find_job_owners(failed_jobs, owners_file=".gitlab/JOBOWNERS"):
    from codeowners import CodeOwners

    with open(owners_file, 'r') as f:
        owners = CodeOwners(f.read())

    owners_to_notify = defaultdict(list)

    for job in failed_jobs:
        # Exclude jobs that were retried and succeeded
        # Also exclude jobs allowed to fail
        if job["status"] == "failed" and not job["allow_failure"]:
            job_owners = owners.of(job["name"])
            # job_owners is a list of tuples containing the type of owner (eg. USERNAME, TEAM) and the name of the owner
            # eg. [('TEAM', '@DataDog/agent-platform')]

            for owner in job_owners:
                owners_to_notify[owner[1]].append(job)

    return owners_to_notify


def base_message(header):
    return """{header} pipeline <{pipeline_url}|{pipeline_id}> for {commit_ref_name} failed.
{commit_title} (<{commit_url}|{commit_short_sha}>) by {author}""".format(
        header=header,
        pipeline_url=os.getenv("CI_PIPELINE_URL"),
        pipeline_id=os.getenv("CI_PIPELINE_ID"),
        commit_ref_name=os.getenv("CI_COMMIT_REF_NAME"),
        commit_title=os.getenv("CI_COMMIT_TITLE"),
        commit_url="{project_url}/commit/{commit_sha}".format(
            project_url=os.getenv("CI_PROJECT_URL"), commit_sha=os.getenv("CI_COMMIT_SHA")
        ),
        commit_short_sha=os.getenv("CI_COMMIT_SHORT_SHA"),
        author=get_git_author(),
    )


def prepare_global_failure_message(header, failed_jobs):
    message = base_message(header)

    message += "\nFailed jobs:"
    for job in failed_jobs:
        # Exclude jobs that were retried and succeeded
        # Also exclude jobs allowed to fail
        if job["status"] == "failed" and not job["allow_failure"]:
            message += "\n - <{url}|{name}> (stage: {stage}, after {retries} retries)".format(
                url=job["url"], name=job["name"], stage=job["stage"], retries=len(job["retry_summary"]) - 1
            )

    return message


def prepare_team_failure_message(header, failed_jobs):
    message = base_message(header)

    message += "\nFailed jobs you own:"
    for job in failed_jobs:
        message += "\n - <{url}|{name}> (stage: {stage}, after {retries} retries)".format(
            url=job["url"], name=job["name"], stage=job["stage"], retries=len(job["retry_summary"]) - 1
        )

    return message


def get_git_author():
    return (
        subprocess.check_output(["git", "show", "-s", "--format='%an'", "HEAD"])
        .decode('utf-8')
        .strip()
        .replace("'", "")
    )


def send_slack_message(recipient, message):
    subprocess.run(["postmessage", recipient, message], check=True)
