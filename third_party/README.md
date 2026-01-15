# Third Party code

All files under this folder are owned by other entitites than DataDog.

While our normal build process is to reference upstream projects by URL of a
release artifact, sometimes this is not feasible.  Typical cases are:

1. The dependency has been abandoned and we need to maintain it ourselves.
1. That we need a small subset of files from an upstream repository ahead of their release.
1. We are actively working to add features to a dependency, and are co-developing those
   along with our own use.

In the first case, this code may be long lived. In all other cases, we expect
to drop the code from this repository when the features are available in an
upstream release.

# Rules

- You must only copy projects that are published under a license that permits this copying.
- All projects must have an explicit owner(s), listed in the file DATADOG.md in the tree.
