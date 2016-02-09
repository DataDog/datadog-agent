# stdlib
from contextlib import nested
from functools import wraps
import logging
import subprocess
import tempfile

# project
from utils.platform import Platform

log = logging.getLogger(__name__)


# FIXME: python 2.7 has a far better way to do this
def get_subprocess_output(command, log, shell=False, stdin=None):
    """
    Run the given subprocess command and return it's output. Raise an Exception
    if an error occurs.
    """

    # Use tempfile, allowing a larger amount of memory. The subprocess.Popen
    # docs warn that the data read is buffered in memory. They suggest not to
    # use subprocess.PIPE if the data size is large or unlimited.
    with nested(tempfile.TemporaryFile(), tempfile.TemporaryFile()) as (stdout_f, stderr_f):
        proc = subprocess.Popen(command,
                                close_fds=not Platform.is_windows(),  # only set to True when on Unix, for WIN compatibility
                                shell=shell,
                                stdin=stdin,
                                stdout=stdout_f,
                                stderr=stderr_f)
        proc.wait()
        stderr_f.seek(0)
        err = stderr_f.read()
        if err:
            log.debug("Error while running {0} : {1}".format(" ".join(command),
                                                             err))

        stdout_f.seek(0)
        output = stdout_f.read()
    return (output, err, proc.returncode)


def log_subprocess(func):
    """
    Wrapper around subprocess to log.debug commands.
    """
    @wraps(func)
    def wrapper(*params, **kwargs):
        fc = "%s(%s)" % (func.__name__, ', '.join(
            [a.__repr__() for a in params] +
            ["%s = %s" % (a, b) for a, b in kwargs.items()]
        ))
        log.debug("%s called" % fc)
        return func(*params, **kwargs)
    return wrapper

subprocess.Popen = log_subprocess(subprocess.Popen)
