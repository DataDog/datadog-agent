import datetime
import functools
import platform
import sys
from time import sleep, time

from tasks.libs.common.color import color_message
from tasks.libs.common.user_interactions import yes_no_question
from tasks.libs.common.utils import DEFAULT_BRANCH

PIPELINE_FINISH_TIMEOUT_SEC = 3600 * 5


class FilteredOutException(Exception):
    pass


def get_running_pipelines_on_same_ref(gitlab, ref, sha=None):
    pipelines = gitlab.all_pipelines_for_ref(ref, sha=sha)

    RUNNING_STATUSES = ["created", "pending", "running"]
    running_pipelines = [pipeline for pipeline in pipelines if pipeline["status"] in RUNNING_STATUSES]

    return running_pipelines


def parse_datetime(dt):
    # before python 3.7, the Z shorthand for UTC timezone was not accepted
    if sys.version_info.major < 3 or sys.version_info.minor < 7:
        if dt.endswith("Z"):
            dt = dt[:-1] + "+00:00"
    return datetime.datetime.strptime(dt, "%Y-%m-%dT%H:%M:%S.%f%z")


def cancel_pipelines_with_confirmation(gitlab, pipelines):
    for pipeline in pipelines:
        commit_author, commit_short_sha, commit_title = get_commit_for_pipeline(gitlab, pipeline['id'])

        print(
            color_message("Pipeline", "blue"),
            color_message(pipeline['id'], "bold"),
            color_message(f"(https://gitlab.ddbuild.io/{gitlab.project_name}/pipelines/{pipeline['id']})", "green"),
        )

        pipeline_creation_date = pipeline['created_at']
        print(
            f"{color_message('Started at', 'blue')} {parse_datetime(pipeline_creation_date).astimezone():%c} ({pipeline_creation_date})"
        )

        print(
            color_message("Commit:", "blue"),
            color_message(commit_title, "green"),
            color_message(f"({commit_short_sha})", "grey"),
            color_message("by", "blue"),
            color_message(commit_author, "bold"),
        )

        if yes_no_question("Do you want to cancel this pipeline?", color="orange", default=True):
            gitlab.cancel_pipeline(pipeline['id'])
            print(f"Pipeline {color_message(pipeline['id'], 'bold')} has been cancelled.\n")
        else:
            print(f"Pipeline {color_message(pipeline['id'], 'bold')} will keep running.\n")


def gracefully_cancel_pipeline(gitlab, pipeline, force_cancel_stages):
    """
    Gracefully cancel pipeline
    - Cancel all the jobs that did not start to run yet
    - Do not cancel jobs containing 'cleanup' in their name
    - Jobs in the stages specified in 'force_cancel_stages' variables will always be canceled even if running
    """

    jobs = gitlab.all_jobs(pipeline["id"])

    for job in jobs:
        if job["stage"] in force_cancel_stages or (
            job["status"] not in ["running", "canceled"] and "cleanup" not in job["name"]
        ):
            gitlab.cancel_job(job["id"])


def trigger_agent_pipeline(
    gitlab,
    ref=DEFAULT_BRANCH,
    release_version_6="nightly",
    release_version_7="nightly-a7",
    branch="nightly",
    deploy=False,
    all_builds=False,
    e2e_tests=False,
    rc_build=False,
    rc_k8s_deployments=False,
):
    """
    Trigger a pipeline on the datadog-agent repositories. Multiple options are available:
    - run a pipeline with all builds (by default, a pipeline only runs a subset of all available builds),
    - run a pipeline with all kitchen tests,
    - run a pipeline with all end-to-end tests,
    - run a deploy pipeline (includes all builds & kitchen tests + uploads artifacts to staging repositories);
    """
    args = {}

    if deploy:
        args["DEPLOY_AGENT"] = "true"

    # The RUN_ALL_BUILDS option can be selectively enabled. However, it cannot be explicitly
    # disabled on pipelines where they're activated by default (default branch & deploy pipelines)
    # as that would make the pipeline fail (some jobs on the default branch and deploy pipelines depend
    # on jobs that are only run if RUN_ALL_BUILDS is true).
    if all_builds:
        args["RUN_ALL_BUILDS"] = "true"

    # End to end tests can be selectively enabled, or disabled on pipelines where they're
    # enabled by default (default branch and deploy pipelines).
    if e2e_tests:
        args["RUN_E2E_TESTS"] = "on"
    else:
        args["RUN_E2E_TESTS"] = "off"

    if release_version_6 is not None:
        args["RELEASE_VERSION_6"] = release_version_6

    if release_version_7 is not None:
        args["RELEASE_VERSION_7"] = release_version_7

    if branch is not None:
        args["BUCKET_BRANCH"] = branch

    if rc_build:
        args["RC_BUILD"] = "true"

    if rc_k8s_deployments:
        args["RC_K8S_DEPLOYMENTS"] = "true"

    print(
        "Creating pipeline for datadog-agent on branch/tag {} with args:\n{}".format(  # noqa: FS002
            ref, "\n".join(f"  - {k}: {args[k]}" for k in args)
        )
    )
    result = gitlab.create_pipeline(ref, args)

    if result and "id" in result:
        return result["id"]

    if result and "filtered out by workflow rules" in result.get("message", {}).get("base", [""])[0]:
        raise FilteredOutException

    raise RuntimeError(f"Invalid response from Gitlab: {result}")


def wait_for_pipeline(gitlab, pipeline_id, pipeline_finish_timeout_sec=PIPELINE_FINISH_TIMEOUT_SEC):
    """
    Follow a given pipeline, periodically checking the pipeline status
    and printing changes to the job statuses.
    """
    commit_author, commit_short_sha, commit_title = get_commit_for_pipeline(gitlab, pipeline_id)

    print(
        color_message(
            "Commit: "
            + color_message(commit_title, "green")
            + color_message(f" ({commit_short_sha})", "grey")
            + " by "
            + color_message(commit_author, "bold"),
            "blue",
        ),
        flush=True,
    )
    print(
        color_message(
            "Pipeline Link: "
            + color_message(f"https://gitlab.ddbuild.io/{gitlab.project_name}/pipelines/{pipeline_id}", "green"),
            "blue",
        ),
        flush=True,
    )

    print(color_message("Waiting for pipeline to finish. Exiting won't cancel it.", "blue"), flush=True)

    f = functools.partial(pipeline_status, gitlab, pipeline_id)

    loop_status(f, pipeline_finish_timeout_sec)

    return pipeline_id


def get_commit_for_pipeline(gitlab, pipeline_id):
    pipeline = gitlab.pipeline(pipeline_id)
    sha = pipeline['sha']
    commit = gitlab.commit(sha)
    return commit['author_name'], commit['short_id'], commit['title']


def loop_status(callable, timeout_sec):
    """
    Utility to loop a function that takes a status and returns [done, status], until done is True.
    """
    start = time()
    status = dict()
    while True:
        done, status = callable(status)
        if done:
            return status
        if time() - start > timeout_sec:
            raise ErrorMsg("Timed out.")
        sleep(10)


def pipeline_status(gitlab, pipeline_id, job_status):
    """
    Checks the pipeline status and updates job statuses.
    """
    jobs = gitlab.all_jobs(pipeline_id)

    job_status = update_job_status(jobs, job_status)

    # Check pipeline status
    pipeline = gitlab.pipeline(pipeline_id)
    pipestatus = pipeline["status"].lower().strip()
    ref = pipeline["ref"]

    if pipestatus == "success":
        print(
            color_message(
                f"Pipeline https://gitlab.ddbuild.io/{gitlab.project_name}/pipelines/{pipeline_id} for {ref} succeeded",
                "green",
            ),
            flush=True,
        )
        notify("Pipeline success", f"Pipeline {pipeline_id} for {ref} succeeded.")
        return True, job_status

    if pipestatus == "failed":
        print(
            color_message(
                f"Pipeline https://gitlab.ddbuild.io/{gitlab.project_name}/pipelines/{pipeline_id} for {ref} failed",
                "red",
            ),
            flush=True,
        )
        notify("Pipeline failure", f"Pipeline {pipeline_id} for {ref} failed.")
        return True, job_status

    if pipestatus == "canceled":
        print(
            color_message(
                f"Pipeline https://gitlab.ddbuild.io/{gitlab.project_name}/pipelines/{pipeline_id} for {ref} was canceled",
                "grey",
            ),
            flush=True,
        )
        notify("Pipeline canceled", f"Pipeline {pipeline_id} for {ref} was canceled.")
        return True, job_status

    if pipestatus not in ["created", "running", "pending"]:
        raise ErrorMsg(f"Error: pipeline status {pipestatus.title()}")

    return False, job_status


def update_job_status(jobs, job_status):
    """
    Updates job statuses and notify on changes.
    """
    notify = {}
    for job in jobs:
        if job_status.get(job['name'], None) is None:
            job_status[job['name']] = job
            notify[job['id']] = job
        else:
            # There are two reasons why we want to notify:
            # - status change on job (when we refresh)
            # - another job with the same name exists (when a job is retried)
            # Check for id to see if we're in the first case.
            old_job = job_status[job['name']]
            if job['id'] == old_job['id'] and job['status'] != old_job['status']:
                job_status[job['name']] = job
                notify[job['id']] = job
            if job['id'] != old_job['id'] and job['created_at'] > old_job['created_at']:
                job_status[job['name']] = job
                # Check if old job already in notification list, to append retry message
                notify_old_job = notify.get(old_job['id'], None)
                if notify_old_job is not None:
                    notify_old_job['retried_old'] = True  # Add message to say the job got retried
                    notify_old_job['retried_created_at'] = job['created_at']
                    notify[old_job['id']] = notify_old_job
                # If not (eg. previous job was notified in last refresh), add retry message to new job
                else:
                    job['retried_new'] = True
                notify[job['id']] = job

    for job in notify.values():
        print_job_status(job)

    return job_status


def print_job_status(job):
    """
    Prints notifications about job changes.
    """

    def print_job(name, stage, color, date, duration, status, link):
        print(
            color_message(
                f"[{date}] Job {name} (stage: {stage}) {status} [job duration: {duration // 60:.0f}m{duration % 60:2.0f}s]\n{link}".strip(),
                color,
            ),
            flush=True,
        )

    def print_retry(name, date):
        print(color_message(f"[{date}] Job {name} was retried", "grey"))

    name = job['name']
    stage = job['stage']
    allow_failure = job['allow_failure']
    duration = job['duration']
    date = job['finished_at']  # Date that is printed in the console log. In most cases, it's when the job finished.
    status = job['status']  # Gitlab job status
    job_status = None  # Status string printed in the console
    link = ''  # Link to the pipeline. Only filled for failing jobs, to be able to quickly go to the failing job.
    color = 'grey'  # Log output color

    # A None duration is set by Gitlab when the job gets canceled before it was started.
    # In that case, set a duration of 0s.
    if duration is None:
        duration = 0

    if status == 'success':
        job_status = 'succeeded'
        color = 'green'
    elif status == 'failed':
        if allow_failure:
            job_status = 'failed (allowed to fail)'
            color = 'orange'
        else:
            job_status = 'failed'
            color = 'red'
            link = f"Link: {job['web_url']}"
            # Only notify on real (not retried) failures
            # Best-effort, as there can be situations where the retried
            # job didn't get created yet
            if job.get('retried_old', None) is None:
                notify("Job failure", f"Job {name} failed.")
    elif status == 'canceled':
        job_status = 'was canceled'
        color = 'grey'
    elif status == 'running':
        job_status = 'started running'
        date = job['started_at']
        color = 'blue'
    else:
        return

    # Some logic to print the retry message in the correct order (before the new job or after the old job)
    if job.get('retried_new', None) is not None:
        print_retry(name, job['created_at'])
    print_job(name, stage, color, date, duration, job_status, link)
    if job.get('retried_old', None) is not None:
        print_retry(name, job['retried_created_at'])


def notify(title, info_text, sound=True):
    """
    Utility to send an OS-level notification. Supported on MacOS and Windows.
    """
    if platform.system() == "Darwin":
        try:
            import objc

            # We could use osascript and this would be far less code but when you click on the notification, it redirects to
            # Script Editor. This is a more ideal way of doing notification as it has no onClick action.
            NSUserNotification = objc.lookUpClass("NSUserNotification")
            NSUserNotificationCenter = objc.lookUpClass("NSUserNotificationCenter")
            notification = NSUserNotification.alloc().init()
            notification.setTitle_(title)
            notification.setInformativeText_(info_text)
            if sound:
                notification.setSoundName_("NSUserNotificationDefaultSoundName")
            NSUserNotificationCenter.defaultUserNotificationCenter().scheduleNotification_(notification)

        except Exception:
            print("Could not send MacOS notification. Run 'pip install pyobjc' to get notifications.")
            pass

    elif platform.system() == "Windows":
        try:
            from win10toast import ToastNotifier

            toaster = ToastNotifier()
            toaster.show_toast(title, info_text, icon_path=None, duration=10)
        except Exception:
            print("Could not send Windows notification. Run 'pip install win10toast' to get notifications.")
            pass


class ErrorMsg(Exception):
    pass
