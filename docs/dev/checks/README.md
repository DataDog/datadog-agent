# Custom checks developer guide

For more informations about what a Custom check is and whether they are a good
fit for your use case, please [refer to the official documentation][custom-checks].

## JMX-based checks
JMX-based checks are executed by a component of the Agent called `jmxfetch`.
Refer to [./jmxfetch.md](./jmxfetch.md) for more.

## Configuration

Every check has its own YAML configuration file. The file has one mandatory key,
`instances` and one optional, `init_config`.


### init_config

This section contains any global configuration options for the check, i.e. any
configuration that all instances of the check can share. Python checks can access
these configuration options via the `self.init_config` dictionary.

There is no required format for the items in `init_config`, but most checks just
use simple key-value configuration, e.g.

Example:
```yaml
init_config:
  default_timeout: 4
  idle_ttl: 300
```

### instances

This section is a list, with each item representing one "instance" — i.e. one
running invocation of the check. For example, when using the HTTP check, you
can configure multiple instances in order to monitor multiple HTTP endpoints:

```yaml
instances:
  - server_url: https://backend1
    user: user1
    password: password
    interval: 60
  - server_url: https://backend2
    token: <SOME_AUTH_TOKEN>
    timeout: 20
```

Each instance, like the `init_config` section, may contain data in any format.
It's up to the check author how to structure configuration data.

Each instances of a check are completely independent from one another and might
run at different intervals.

## Anatomy of a Python Check

Same as any built-in integration, a Custom Check consists of a Python class that
inherits from `AgentCheck` and implements the `check` method:

```python
from datadog_checks.checks import AgentCheck

class MyCheck(AgentCheck):
    def check(self, instance):
        # Collect metrics, emit events, submit service checks,
        # ...
```

The Agent creates an object of type `MyCheck` for each element contained in the
`instances` sequence within the corresponding config file:

```
instances:
  - host: localhost
    port: 6379

  - host: example.com
    port: 6379
```

Any mapping contained in `instances` is passed to the `check` method through the
named parameter `instance`. The `check` method is invoked at every run of the
[collector][collector].

The `AgentCheck` base class provides several useful attributes and methods,
refer to the [Python docs][datadog_checks_base] and the developer
[documentation pages][developer_docs] for more details.

### Running subprocesses

Due to the Python interpreter being embedded in an inherently multi-threaded environment (the go runtime)
there are some limitations to the way Python Checks can run subprocesses.

To run a subprocess from your check, use the `get_subprocess_output` function
provided in `datadog_checks.utils.subprocess_output`:

```python
from datadog_checks.utils.subprocess_output import get_subprocess_output

class MyCheck(AgentCheck):
    def check(self, instance):
    # [...]
    out, err, retcode = get_subprocess_output(cmd, self.log, raise_on_empty_output=True)
```

Using the `subprocess` and `multiprocessing` modules provided by the Python standard library is _not
supported_, and may result in your Agent crashing and/or creating processes that remain in a stuck or zombie
state.

### Custom built-in modules

A set of Python modules is provided capable to interact with a running Agent at
a quite low level. These modules are built-in but only available in the embedded
CPython interpreter within a running Agent and are mostly used in the `AgentCheck`
base class which exposes convenient wrappers to be used in integrations and custom
checks code.

**These modules should never be used directly.**

- [_util](builtins/_util.md)
- [aggregator](builtins/aggregator.md)
- [containers](builtins/containers.md)
- [datadog_agent](builtins/datadog_agent.md)
- [kubeutil](builtins/kubeutil.md)
- [tagger](builtins/tagger.md)
- [util](builtins/util.md)

[custom-checks]: https://docs.datadoghq.com/developers/write_agent_check/?tab=agentv6
[collector]: /pkg/collector
[datadog_checks_base]: https://datadoghq.dev/integrations-core/base/about/
[developer_docs]: https://docs.datadoghq.com/developers/

## Running checks with a local Agent Build
### Custom Checks
Scenario: You have implemented a custom check called `hello_world` and you would
like to run this with a local Agent build.

Example contents of `hello_world.py`:
```python
from datadog_checks.checks import AgentCheck

class MyCheck(AgentCheck):
    def check(self, instance):
        self.gauge('hello.world', 1.23, tags=['foo:bar'])
```

1. Place the configuration file `hello_world.yaml` in the `dev/dist/conf.d/` folder.
1. Place your Python code in the `dev/dist/` folder.
1. Run `deva agent.build` as usual. This step copies the contents
   of `dev/dist` into `bin/agent/dist`, which is where the Agent looks
   for your code.

The resulting directory structure should look like:
```
dev/dist
├── conf.d
│   └── hello_world.yaml
├── hello_world.py
└── README.md
```

This is sufficient to have the Agent correctly discover both the code and the
configuration, but in order to actually run this, we must [discuss
dependencies](https://xkcd.com/1987/)

### Python Check Dependencies
When `hello_world.py` runs, it loads in the `AgentCheck` class which provides
some default functionality (eg, the `self.gauge` method)

In order for python to find this package, we must do two things:
1. Install the `integrations-core` dependencies
2. Set the `PYTHONPATH` env var correctly to find those dependencies

`datadog-checks-base` must be installed as a pre-req, and you can install via a
`--user` install or a `virtualenv` install, as long as you set your `PYTHONPATH`
correctly.

#### Example for virtualenv
(see also the notes in [../agent_dev_env.md](../agent_dev_env.md#python-dependencies)):

1. `python3 -m pip install virtualenv`
1. `virtualenv $GOPATH/src/github.com/DataDog/datadog-agent/venv`
    1. You may need a `-p python3` argument if the system has both `python2` and
       `python3` bins.
1. Activate the virtualenv (OS-dependent)
1. `python3 -m pip install '/path/to/integrations-core/datadog_checks_base[deps]'`
1. `PYTHONPATH="$PWD/venv/lib/python3.10/site-packages:$PYTHONPATH" deva agent.run`


#### Example for user install
1. `python3 -m pip install --user '/path/to//integrations-core/datadog_checks_base[deps]'`
1. `PYTHONPATH="$HOME/.local/lib/python3.10/site-packages:$PYTHONPATH" deva agent.run`

#### Getting the right `PYTHONPATH`
You want the `site-packages` directory that the `datadog_checks_base` got
installed to.

`python -m site` is very helpful to reference for the paths your current python
is looking at.

### Standard Checks
There are a number of checks that ship with a full build of the Agent, but when
you're developing against a local build, you will not get any of these Python
checks out of the box, you must install them.

> Note the following instructions install Python packages to the Python user
> install directory. Please see above for instructions on virtualenv

The list of checks currently shipping with the Agent lives in the
[integrations-core repo
here](https://github.com/DataDog/integrations-core/blob/master/requirements-agent-release.txt)

Scenario: You want to test the `redisdb` check:

1. Clone `integrations-core` locally. (Optionally, check out the git tag
   corresponding to the version you want to test.)
2. From the `integrations-core/` directory, install `datadog-checks-base` as a pre-req.
    ```
    python3 -m pip install --user './datadog_checks_base[deps]'
    ```
3. Install the `redisdb` check. From inside the `integrations-core`
   checkout:
    ```
    python3 -m pip install --user ./redisdb
    ```

4. (Optional for some checks). Some checks have dependencies on other Python modules
   that must be installed alongside the Python check. `redisdb` is one check that _does_ have
   dependencies, specifically on the open source `redisdb` package. In this case, we need to
   install the `deps` explicitly.
   ```
   python3 -m pip install --user './redisdb[deps]'
   ```

That's it! Your local build should now have the correct packages to be able to
run the `redisdb` check.

(You may need to set the correct `PYTHONPATH` when running the Agent, depending on your environment and which python is being used. There are instructions above.)

#### "What is this `[deps]` thing?"
The `[deps]` at the end of the package name instructs pip to install the
requirements that match the `deps` "extra". Without this, you'll see errors when running the Agent
about missing dependencies.

View all the possible 'extras' with:
```
python3 -m pip install --no-warn-script-location --user --dry-run --ignore-installed -qqq --report - datadog-checks-base | jq '.install[] | select(.metadata.name=="datadog-checks-base") | .metadata.provides_extra'
```

View all the requirements (which shows which "extra" will trigger that
dependency to be installed) with:
```
python3 -m pip install --no-warn-script-location --user --dry-run --ignore-installed -qqq --report - datadog-checks-base | jq '.install[] | select(.metadata.name=="datadog-checks-base") | .metadata.requires_dist'
```

#### "Which Python binary is my Agent build using?"
Assuming you're not doing a full omnibus build, you won't have an embedded
Python binary in your local build, so your local build will need to choose a Python
binary from your system to use.

By default, the Agent chooses the first python binary (`python` or `python3`
depending on your configuration) from the `PATH` that is used to run the Agent.

> There are some environment variables that can change the way the binary is
> chosen, for the full story see the function `resolvePythonExecPath`
> in `pkg/collector/python/init.go`.

### "Could not initialize Python"
Out of the box, after an `deva agent.build`, you may see the following error on
Linux when trying to run the resulting `agent` binary:

`Could not initialize Python: could not load runtime python for version 3: Unable to open three library: libdatadog-agent-three.so: cannot open shared object file: No such file or directory`

To solve on Linux, use the loader env var `LD_LIBRARY_PATH`.
For example, `LD_LIBRARY_PATH=$PWD/dev/lib ./bin/agent/agent run`

Why is this needed? This is due to a combination of the way `rtloader` works
and how library loading works on Linux.

The very simplified summary is that `libdatadog-agent-rtloader.so` attempts to load
`libdatadog-agent-three.so` via `dlopen`, however `libdatadog-agent-rtloader`
does not have its `RUNPATH` set correctly in local builds, so `dlopen` is unable to find
`libdatadog-agent-three.so`. Using `LD_LIBRARY_PATH` instructs `dlopen` where to
search for libraries, so using it sidesteps this issue.
