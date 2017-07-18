# Py-Launcher

The Datadog Agent inject multiple synthetic modules in the Python namespace.
This allows the agent to run integration in Python. **py-launcher** run a
python script as it was run by the agent. This includes synthetic modules,
logging facilities, configuration setup, ...

**py-launcher** is mainly intended for testing purposes.

## Using

Running a script with no configuration file:
```
py-launcher -py script.py
```

Running a script with a specific configuration file:
```
py-launcher -conf datadog.yaml -py test.py
```

Passing option to the python script
```
py-launcher -py test.py -- --some-option=true -s -v
```

## Exiting in the python script

Calling `sys.exit` in the python script will cause **py-launcher** to exit
also. The value given to `sys.exit` will be the exit status of **py-launcher**.

This could be useful for testing script:

Python script:
```
import sys
import nose
from nose.tools import assert_equals


def test_func():
    assert_equals(1, 2)

if __name__ == '__main__':
    if not nose.run(defaultTest=__name__):
        sys.exit(1)
```

Usage:
```
$> ./py-launcher -py /tmp/test.py -- -v -s; echo "Result: $?"
__main__.test_func ... FAIL

======================================================================
FAIL: __main__.test_func
----------------------------------------------------------------------
Traceback (most recent call last):
  File "/usr/lib/python2.7/dist-packages/nose/case.py", line 197, in runTest
    self.test(*self.arg)
  File "/tmp/test.py", line 7, in test_func
    assert_equals(1, 2)
AssertionError: 1 != 2

----------------------------------------------------------------------
Ran 1 test in 0.000s

FAILED (failures=1)
Result: 1
```
