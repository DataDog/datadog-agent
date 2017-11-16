# package `collector`

This package is responsible for providing any resource and functionality needed in a check
life cycle: retrieving a configuration, create a check instance, schedule a check, running
a check are all operations implemented in this package or one of the subpackages.

## Metadata

All the facilities to compute and post metadata informations to the backend live here, in
analogy with how this worked in the Python agent. Should this code be used by any other
softwares than the Agent itself at some point, the `metadata` package could be moved out
of the `collector` folder and made more visible.

For further details, please refer to the specific READMEs:

* [check](check/README.md)
* [corechecks](corechecks/README.md)
* [metadata](metadata/README.md)
* [providers](providers/README.md)
* [py](py/README.md)
* [runner](runner/README.md)
* [scheduler](scheduler/README.md)
