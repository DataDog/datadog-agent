# Third Party code

All files under this folder are owned by other entitites than DataDog.

While normally reference upstream projects by URL of a release artifact,
sometimes this is not feasible.  Typical cases are:

1. the dependency has been abandoned and we need to maintain it ourselves.
1. that we need a small subset of files from an upstream repository ahead of release.
1. we are actively working to add features to a dependency, and are co-developing them
   along with out own uses. The intent is that we will upstream our fixes and
   remove the dependency.

In the first case, this code may be long lived. In all other cases, we expect
to drop the code from this repository when the features are available in an
upstream release.

# Rules

- You must only copy projects that are published under a license that permits this copying.
- All projects must have an explicit owner(s), list in the file DATADOG.md in the tree.
