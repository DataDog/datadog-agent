# package `diagnose`

This package is used to register and run useful connectivity diagnosis on the agent.

## Running all diagnosis

You can run all registered diagnosis with the `diagnose` command on the agent

The `flare` command will also run registered diagnosis and output them in a `diagnose.log` file.

## Registering a new diagnosis

A diagnosis is a function defined as follow `type Diagnosis func() error`. The presence or not of an `error` will define if the diagnosis has failed or not.

Registering a new diagnosis is pretty straightforward just call the `diagnosis.Register(name string, d Diagnosis)` method. One preferred way to do this is to call it from the `init()` function of your package, so that it's automatically registered if your package is included in the agent.

Example output for a failed check:

```
=== Running <check name> ===
<additional debug logs>
[ERROR] <printed returned error> - <timestamp>
===> FAIL
```

The diagnosis output is leveraging the log system, so make sure the functions you call from your diagnosis are logging pertinent information.
