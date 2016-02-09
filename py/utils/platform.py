# stdlib
import sys

# project
from utils.dockerutil import get_client

_is_ecs = None

class Platform(object):
    """
    Return information about the given platform.
    """
    @staticmethod
    def is_darwin(name=None):
        name = name or sys.platform
        return 'darwin' in name

    @staticmethod
    def is_mac(name=None):
        return Platform.is_darwin(name)

    @staticmethod
    def is_freebsd(name=None):
        name = name or sys.platform
        return name.startswith("freebsd")

    @staticmethod
    def is_linux(name=None):
        name = name or sys.platform
        return 'linux' in name

    @staticmethod
    def is_bsd(name=None):
        """ Return true if this is a BSD like operating system. """
        name = name or sys.platform
        return Platform.is_darwin(name) or Platform.is_freebsd(name)

    @staticmethod
    def is_solaris(name=None):
        name = name or sys.platform
        return name == "sunos5"

    @staticmethod
    def is_unix(name=None):
        """ Return true if the platform is a unix, False otherwise. """
        name = name or sys.platform
        return (
            Platform.is_darwin()
            or Platform.is_linux()
            or Platform.is_freebsd()
        )

    @staticmethod
    def is_win32(name=None):
        name = name or sys.platform
        return name == "win32"

    @staticmethod
    def is_windows(name=None):
        return Platform.is_win32(name)

    @staticmethod
    def is_ecs_instance():
        """Return True if the agent is running in an ECS instance, False otherwise."""
        global _is_ecs
        if _is_ecs is not None:
            return _is_ecs

        try:
            client = get_client()
            containers = client.containers()
            for co in containers:
                if '/ecs-agent' in co.get('Names', ''):
                    _is_ecs = True
                    return True
        except Exception:
            pass

        _is_ecs = False
        return False
