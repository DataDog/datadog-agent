This folder contains templates that are used by `new-bundle` and `new-component` tasks that are defined in [components.py](../components.py).

These templates are used to generate Golang files when scaffolding a new bundle or component. They contain variables, defined as `${VAR_NAME}`, which
are substituted with the `string.Template` python class.