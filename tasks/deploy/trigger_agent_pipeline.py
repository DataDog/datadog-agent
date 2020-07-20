import platform
import functools

from .gitlab import Gitlab
from .color import color_str

from time import sleep, time

PIPELINE_FINISH_TIMEOUT_SEC = 3600 * 5


def trigger_agent_pipeline(
    ref="master",
    release_version_6="nightly",
    release_version_7="nightly-a7",
    branch="nightly",
    windows_update_latest=True,
):
    args = {}

    args["DEPLOY_AGENT"] = "true"

    # The build tag appended to all released binaries
    if release_version_6 is not None:
        args["RELEASE_VERSION_6"] = release_version_6

    # Override the environment to release the binaries to (prod or staging)
    if release_version_7 is not None:
        args["RELEASE_VERSION_7"] = release_version_7

    # Override the environment to release the binaries to (prod or staging)
    if branch is not None:
        args["DEB_RPM_BUCKET_BRANCH"] = branch

    # Override the environment to release the binaries to (prod or staging)
    if windows_update_latest is not None:
        args["WINDOWS_DO_NOT_UPDATE_LATEST"] = str(not windows_update_latest).lower()

    print(
        "Creating pipeline for datadog-agent on branch {} with args:\n{}".format(
            ref, "\n".join(["  - {}: {}".format(k, args[k]) for k in args])
        )
    )
    result = Gitlab().create_pipeline("DataDog/datadog-agent", ref, args)

    if result and "id" in result:
        return result["id"]
    raise RuntimeError("Invalid response from Gitlab: {}".format(result))


def wait_for_pipeline(
    pipeline_id=None, pipeline_finish_timeout_sec=PIPELINE_FINISH_TIMEOUT_SEC,
):
    print_pipeline_link("DataDog/datadog-agent", pipeline_id)

    status("Waiting for pipeline to finish. Exiting won't cancel it.")

    wait_for_pipeline_id("DataDog/datadog-agent", pipeline_id, pipeline_finish_timeout_sec)

    return pipeline_id


def wait_for_pipeline_id(
    proj, pipeline_id, timeout_sec, skip_wait_coverage=False, wait_for_job="",
):
    gitlab = Gitlab()
    gitlab.test_project_found(proj)

    ref = gitlab.pipeline(proj, pipeline_id).get('ref', '')
    f = functools.partial(pipeline_status, gitlab, proj, pipeline_id,)

    loop(f, ref, timeout_sec)


def loop(callable, ref, timeout_sec):
    start = time()
    job_status = dict()
    while True:
        res, job_status = callable(job_status, ref)
        if res:
            return res
        if time() - start > timeout_sec:
            raise ErrorMsg("Timed out.")
        sleep(10)


def pipeline_status(gitlab, proj, pipeline_id, job_status, ref):
    jobs = []
    page = 1

    # Go through all pages
    results = gitlab.jobs(proj, pipeline_id, page)
    while len(results) != 0:
        jobs.extend(results)
        results = gitlab.jobs(proj, pipeline_id, page)
        page += 1

    job_status = update_job_status(jobs, job_status)
    # check pipeline status
    pipestatus = gitlab.pipeline(proj, pipeline_id)["status"].lower().strip()

    if pipestatus == "success":
        print(
            color_str(
                "Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} for {} succeeded".format(proj, pipeline_id, ref),
                "green",
            )
        )
        return True, job_status

    if pipestatus == "failed":
        print(
            color_str(
                "Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} for {} failed".format(proj, pipeline_id, ref), "red"
            )
        )
        notify("Pipeline failure", "Pipeline {} for {} failed.".format(pipeline_id, ref))
        return True, job_status

    if pipestatus == "canceled":
        print(
            color_str(
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
    notify = {}
    for job in jobs:
        if job_status.get(job['name'], None) is None:
            job_status[job['name']] = job
            # If it's already finished, add it to the jobs to print
            if job['status'] in ['running', 'success', 'canceled', 'failed']:
                notify[job['id']] = job
        else:
            # There are two reasons why we want tp notify:
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
    def print_job(name, stage, color, finish_date, duration, status):
        print(
            color_str(
                "[{finish_date}] Job {name} (stage: {stage}) {status} [job duration: {m:.0f}m{s:2.0f}s]".format(
                    name=name,
                    stage=stage,
                    finish_date=finish_date,
                    m=(duration // 60),
                    s=(duration % 60),
                    status=status,
                ),
                color,
            )
        )

    def print_retry(name, date):
        print(color_str("[{date}] Job {name} was retried".format(date=date, name=name,), "grey",))

    name = job['name']
    stage = job['stage']
    date = job['finished_at']
    allow_failure = job['allow_failure']
    duration = job['duration']
    status = job['status']

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
            # Only notify on real (not retried) failures
            # Best-effort, as there can be situations where the retried
            # job didn't get created yet
            if job.get('retried_old', None) is None:
                notify("Job failure", "Job {} failed.".format(name))
    elif status == 'canceled':
        job_status = 'canceled'
        color = 'grey'
    elif status == 'running':
        job_status = 'started running'
        date = job['started_at']
        color = 'blue'
    else:
        return False

    if job.get('retried_new', None) is not None:
        print_retry(name, job['created_at'])
    print_job(name, stage, color, date, duration, job_status)
    if job.get('retried_old', None) is not None:
        print_retry(name, job['retried_created_at'])


def print_pipeline_link(project, pipeline_id):
    status(
        "Pipeline Link: "
        + color_str("https://gitlab.ddbuild.io/{}/pipelines/{}".format(project, pipeline_id), "green",)
    )


def status(msg):
    print(color_str(msg, "blue"))


def notify(title, info_text, sound=True):
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
