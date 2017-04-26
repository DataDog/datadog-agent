from collections import deque
import logging
import os
import signal
import threading
import time
import traceback

# Not available on Windows
try:
    import resource
except ImportError:
    resource = None

try:
    import psutil
except ImportError:
    psutil = None

from utils.platform import Platform

log = logging.getLogger(__name__)


class Watchdog(object):
    def destruct(self):
        raise NotImplementedError('Subclasses must override')

    def reset(self):
        raise NotImplementedError('Subclasses must override')

    def watch(self):
        raise NotImplementedError('Subclasses must override')

    @classmethod
    def create(cls, duration, max_resets=None):
        if Platform.is_windows():
            return WatchdogWindows(duration)
        return WatchdogPosix(duration, max_resets=max_resets)


class WatchdogWindows(Watchdog, threading.Thread):
    """ Simple watchdog for Windows (relies on psutil) """
    def __init__(self, duration):
        self._duration = int(duration)

        threading.Thread.__init__(self)
        self.tlock = threading.RLock()
        self.reset()
        self.start()

    def destruct(self):
        try:
            log.error("Self-destructing...")
            log.error(traceback.format_exc())
        finally:
            # This will kill the current process including the Watchdog's thread
            psutil.Process().kill()

    def reset(self):
        log.debug("Resetting watchdog for %d", self._duration)
        with self.tlock:
            self.expire_at = time.time() + self._duration

    def watch(self):
        while True:
            if time.time() > self.expire_at:
                self.destruct()
            time.sleep(self._duration/20)

    def run(self):
        self.watch()


class WatchdogPosix(object):
    """
    Simple signal-based watchdog. Restarts the process when:
    * no reset was made for more than a specified duration
    * (optional) a suspicious high activity is detected, i.e. too many resets for a given timeframe.

    **Warning**: Not thread-safe.
    Can only be invoked once per process, so don't use with multiple threads.
    If you instantiate more than one, you're also asking for trouble.
    """
    # Activity history timeframe
    _RESTART_TIMEFRAME = 60

    def __init__(self, duration, max_resets=None):
        # Set the duration
        self._duration = int(duration)
        signal.signal(signal.SIGALRM, WatchdogPosix.self_destruct)

        # Set high activity monitoring
        self._restarts = deque([])
        self._max_resets = max_resets

    @staticmethod
    def self_destruct(signum, frame):
        """
        Kill the process. It will be eventually restarted.
        """
        try:
            log.error("Self-destructing...")
            log.error(traceback.format_exc())
        finally:
            os.kill(os.getpid(), signal.SIGKILL)

    def destruct(self):
        WatchdogPosix.self_destruct(None, None)

    def _is_frenetic(self):
        """
        Detect suspicious high activity, i.e. the number of resets exceeds the maximum limit set
        on the watchdog timeframe.
        Flush old activity history
        """
        now = time.time()
        while(self._restarts and self._restarts[0] < now - self._RESTART_TIMEFRAME):
            self._restarts.popleft()

        return len(self._restarts) > self._max_resets

    def reset(self):
        """
        Reset the watchdog state, i.e.
        * re-arm alarm signal
        * (optional) save reset history, flush old entries and check frequency
        """
        # Check activity
        if self._max_resets:
            self._restarts.append(time.time())
            if self._is_frenetic():
                self.destruct()

        # Re arm alarm signal
        log.debug("Resetting watchdog for %d", self._duration)
        signal.alarm(self._duration)
