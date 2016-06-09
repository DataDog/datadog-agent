from threading import Thread
import functools

_thread_by_func = {}


class TimeoutException(Exception):
    """
    Raised when a function runtime exceeds the limit set.
    """
    pass


class ThreadMethod(Thread):
    """
    Descendant of `Thread` class.

    Run the specified target method with the specified arguments.
    Store result and exceptions.

    From: https://code.activestate.com/recipes/440569/
    """
    def __init__(self, target, args, kwargs):
        Thread.__init__(self)
        self.setDaemon(True)
        self.target, self.args, self.kwargs = target, args, kwargs
        self.start()

    def run(self):
        try:
            self.result = self.target(*self.args, **self.kwargs)
        except Exception, e:
            self.exception = e
        else:
            self.exception = None


def timeout(timeout):
    """
    A decorator to timeout a function. Decorated method calls are executed in a separate new thread
    with a specified timeout.
    Also check if a thread for the same function already exists before creating a new one.

    Note: Compatible with Windows (thread based).
    """
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            key = "{0}:{1}:{2}:{3}".format(id(func), func.__name__, args, kwargs)

            if key in _thread_by_func:
                # A thread for the same function already exists.
                worker = _thread_by_func[key]
            else:
                worker = ThreadMethod(func, args, kwargs)
                _thread_by_func[key] = worker

            worker.join(timeout)
            if worker.is_alive():
                raise TimeoutException()

            del _thread_by_func[key]

            if worker.exception:
                raise worker.exception
            else:
                return worker.result

        return wrapper
    return decorator
