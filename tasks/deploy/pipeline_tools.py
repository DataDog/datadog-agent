import functools
import platform
from time import sleep, time

from .color import color_message
from .gitlab import Gitlab

PIPELINE_FINISH_TIMEOUT_SEC = 3600 * 5


def trigger_agent_pipeline(
    ref="master", release_version_6="nightly", release_version_7="nightly-a7", branch="nightly", deploy=True
):
    """
    Trigger a pipeline to deploy an Agent to staging repos
    (as specified with the DEPLOY_AGENT arg).
    """
    args = {}

    if deploy:
        args["DEPLOY_AGENT"] = "true"

    if release_version_6 is not None:
        args["RELEASE_VERSION_6"] = release_version_6

    if release_version_7 is not None:
        args["RELEASE_VERSION_7"] = release_version_7

    if branch is not None:
        args["DEB_RPM_BUCKET_BRANCH"] = branch

    print(
        "Creating pipeline for datadog-agent on branch/tag {} with args:\n{}".format(
            ref, "\n".join(["  - {}: {}".format(k, args[k]) for k in args])
        )
    )
    result = Gitlab().create_pipeline("DataDog/datadog-agent", ref, args)

    if result and "id" in result:
        return result["id"]
    raise RuntimeError("Invalid response from Gitlab: {}".format(result))


def wait_for_pipeline(
    project, pipeline_id, pipeline_finish_timeout_sec=PIPELINE_FINISH_TIMEOUT_SEC,
):
    """
    Follow a given pipeline, periodically checking the pipeline status
    and printing changes to the job statuses.
    """

    gitlab = Gitlab()

    # Check that the project can be found (if not, we probably don't have enough permissions)
    gitlab.test_project_found(project)

    commit_author, commit_short_sha, commit_title = get_commit_for_pipeline(gitlab, project, pipeline_id)

    print(
        color_message(
            "Commit: "
            + color_message(commit_title, "green")
            + color_message(" ({})".format(commit_short_sha), "grey")
            + " by "
            + color_message(commit_author, "bold"),
            "blue",
        )
    )
    print(
        color_message(
            "Pipeline Link: "
            + color_message("https://gitlab.ddbuild.io/{}/pipelines/{}".format(project, pipeline_id), "green",),
            "blue",
        )
    )

    print(color_message("Waiting for pipeline to finish. Exiting won't cancel it.", "blue"))

    f = functools.partial(pipeline_status, gitlab, project, pipeline_id,)

    loop_status(f, pipeline_finish_timeout_sec)

    return pipeline_id


def get_commit_for_pipeline(gitlab, project, pipeline_id):
    pipeline = gitlab.pipeline(project, pipeline_id)
    sha = pipeline['sha']
    commit = gitlab.commit(project, sha)
    return commit['author_name'], commit['short_id'], commit['title']


def loop_status(callable, timeout_sec):
    """
    Utility to loop a function that takes and returns a status, until it returns True.
    """
    start = time()
    status = dict()
    while True:
        res, status = callable(status)
        if res:
            return res
        if time() - start > timeout_sec:
            raise ErrorMsg("Timed out.")
        sleep(10)


def pipeline_status(gitlab, proj, pipeline_id, job_status):
    """
    Checks the pipeline status and updates job statuses.
    """

    jobs = []
    page = 1

    # Go through all pages
    results = gitlab.jobs(proj, pipeline_id, page)
    while len(results) != 0:
        jobs.extend(results)
        results = gitlab.jobs(proj, pipeline_id, page)
        page += 1

    job_status = update_job_status(jobs, job_status)

    # Check pipeline status
    pipeline = gitlab.pipeline(proj, pipeline_id)
    pipestatus = pipeline["status"].lower().strip()
    ref = pipeline["ref"]

    if pipestatus == "success":
        print(
            color_message(
                "Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} for {} succeeded".format(proj, pipeline_id, ref),
                "green",
            )
        )
        notify("Pipeline success", "Pipeline {} for {} succeeded.".format(pipeline_id, ref))
        return True, job_status

    if pipestatus == "failed":
        print(
            color_message(
                "Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} for {} failed".format(proj, pipeline_id, ref), "red"
            )
        )
        notify("Pipeline failure", "Pipeline {} for {} failed.".format(pipeline_id, ref))
        return True, job_status

    if pipestatus == "canceled":
        print(
            color_message(
                "Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} for {} was canceled".format(proj, pipeline_id, ref),
                "grey",
            )
        )
        notify("Pipeline canceled", "Pipeline {} for {} was canceled.".format(pipeline_id, ref))
        return True, job_status

    if pipestatus not in ["created", "running", "pending"]:
        raise ErrorMsg("Error: pipeline status {}".format(pipestatus.title()))

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
                "[{date}] Job {name} (stage: {stage}) {status} [job duration: {m:.0f}m{s:2.0f}s]\n{link}".format(
                    name=name, stage=stage, date=date, m=(duration // 60), s=(duration % 60), status=status, link=link,
                ).strip(),
                color,
            )
        )

    def print_retry(name, date):
        print(color_message("[{date}] Job {name} was retried".format(date=date, name=name,), "grey",))

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
            link = "Link: {}".format(job['web_url'])
            # Only notify on real (not retried) failures
            # Best-effort, as there can be situations where the retried
            # job didn't get created yet
            if job.get('retried_old', None) is None:
                notify("Job failure", "Job {} failed.".format(name))
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
