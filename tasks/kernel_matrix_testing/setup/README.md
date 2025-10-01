# KMT setup code

This folder contains the code for the KMT environment setup and checking. It's organized in the following way:

* `common.py`: Common requirements for all platforms
* `<platform>.py`: Platform-specific requirements
* `{common,<platform>}_localvms.py`: Local VM-specific requirements, for either all or a specific platform

These files contain classes that inherit from `Requirement` and implement the `check` method. The `check` method is used to check if the requirement is met, and if not, it will return a `RequirementState` object with the status of the requirement. If the `fix` parameter is `True`, the `check` method will also try to fix the requirement if possible.

Any new requirements should be added to the `get_requirements` function in the corresponding file. While this could be done in the `__init__.py` file by automatically importing all the requirements, doing it explicitly allows vulture to better check for unused code.

The main entry points for the module are the following two functions in `__init__.py`:

* `get_requirements` function, which returns a list of `Requirement` objects based on the platform and the `remote_setup_only` parameter.
* `check_requirements` function, which checks all the requirements and returns a boolean indicating if the KMT setup is correct or not.

## "FAIL" and "WARN"

The `RequirementState` class has a `state` attribute that can be one of `Status.OK`, `Status.WARN`, or `Status.FAIL`.

* `Status.OK`: The requirement is met.
* `Status.WARN`: The requirement is not completely met, but KMT _might_ still work.
* `Status.FAIL`: The requirement is not met and KMT _will not_ work.

The distinction is important, as warning statuses will not cause the `check_requirements` function to return `False`.
