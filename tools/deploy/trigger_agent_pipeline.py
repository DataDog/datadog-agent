import platform

from gitlab import Gitlab
from color import color_str


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
        args["WINDOWS_DO_NOT_UPDATE_LATEST"] = str(not windows_update_latest)

    print(
        "Creating pipeline for datadog-agent on branch {} with args:\n{}".format(
            ref, "\n".join(["  - {}: {}".format(k, args[k]) for k in args])
        )
    )
    result = Gitlab().create_pipeline("22", ref, args)

    if result and "id" in result:
        return result["id"]
    raise RuntimeError("Invalid response from Gitlab: {}".format(result))


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
