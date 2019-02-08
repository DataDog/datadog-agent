import time


def wait_until(someaction, timeout, period=0.25, *args, **kwargs):
    mustend = time.time() + timeout
    while True:
        try:
            someaction(*args, **kwargs)
            return
        except:
            if time.time() >= mustend:
                print("Waiting timed out after %d" % timeout)
                raise
            time.sleep(period)
