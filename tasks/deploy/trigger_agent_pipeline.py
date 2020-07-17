import platform
import functools
import sys

from .gitlab import Gitlab
from .color import color_str

from time import sleep, time

PIPELINE_FINISH_TIMEOUT_SEC = 3600 * 4


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
    result = Gitlab().create_pipeline("22", ref, args)

    if result and "id" in result:
        return result["id"]
    raise RuntimeError("Invalid response from Gitlab: {}".format(result))


def loop(callable, timeout_sec):
    need_newline = False
    start = time()
    job_status = dict()
    try:
        while True:
            res, job_status = callable(job_status)
            if res:
                return res
            if time() - start > timeout_sec:
                raise ErrorMsg("Timed out.")

            sys.stdout.flush()
            need_newline = True
            sleep(3)
    finally:
        if need_newline:
            print("")


def wait_for_pipeline_id(
    proj, pipeline_id, timeout_sec, commit=None, skip_wait_coverage=False, wait_for_job="",
):
    gitlab = Gitlab()
    gitlab.test_project_found(proj)

    f = functools.partial(pipeline_status, gitlab, proj, pipeline_id,)

    loop(f, timeout_sec)


def latest_job_from_name(jobs, job_name):
    relevant_jobs = [job for job in jobs if job["name"] == job_name]

    return sorted(relevant_jobs, key=lambda x: x["created_at"], reverse=True)[0]


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

    name = job['name']
    stage = job['stage']
    finish_date = job['finished_at']
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
    elif status == 'canceled':
        job_status = 'canceled'
        color = 'grey'

    print_job(name, stage, color, finish_date, duration, job_status)


def update_job_status(jobs, job_status):
    for job in jobs:
        if job['status'] in ['success', 'canceled', 'failed']:
            if job_status.get(job['name'], None) is None:
                job_status[job['name']] = (job['status'], job['allow_failure'])
                print_job_status(job)
    return job_status


def pipeline_status(gitlab, proj, pipeline_id, job_status):
    jobs = gitlab.jobs(proj, pipeline_id)

    job_status = update_job_status(jobs, job_status)
    # check pipeline status
    pipestatus = gitlab.pipeline(proj, pipeline_id)["status"].lower().strip()

    if pipestatus == "success":
        return True

    if pipestatus == "failed":
        for job in jobs:
            if job["status"] in ["failed", "canceled"]:
                url = "https://gitlab.ddbuild.io/{}/-/jobs/{}".format(proj, job["id"])
                raise ErrorMsg("Job {}: {}".format(job["status"], url))
        raise ErrorMsg(
            "Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} failed, despite no failed or canceled jobs...".format(
                proj, pipeline_id
            )
        )

    if pipestatus == "canceled":
        raise ErrorMsg("Pipeline https://gitlab.ddbuild.io/{}/pipelines/{} was canceled.".format(proj, pipeline_id))

    if pipestatus not in ["created", "running", "pending"]:
        raise ErrorMsg("Error: pipeline status {}".format(pipestatus.title()))

    return False, job_status


def print_pipeline_link(project, pipeline_id):
    status(
        "Pipeline Link: " + color_str("https://gitlab.ddbuild.io/%s/pipelines/%d" % (project, pipeline_id), "green",)
    )


def wait_for_pipeline(
    pipeline_id, pipeline_finish_timeout_sec=PIPELINE_FINISH_TIMEOUT_SEC,
):
    start = time()

    print_pipeline_link("22", pipeline_id)

    status("Waiting for pipeline to finish. Exiting won't cancel it.")

    wait_for_pipeline_id("22", pipeline_id, pipeline_finish_timeout_sec)

    wait_sec = time() - start  # not called duration because pipeline may already have been running
    print(color_str("Done in %ds!" % (wait_sec,), "green"))

    return pipeline_id


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
            pass

    elif platform.system() == "Windows":
        try:
            from win10toast import ToastNotifier

            toaster = ToastNotifier()
            toaster.show_toast(title, info_text, icon_path=None, duration=10)
        except Exception:
            pass
    ## Always do console notification
    print("%s: %s" % (title, info_text))


class ErrorMsg(Exception):
    pass
