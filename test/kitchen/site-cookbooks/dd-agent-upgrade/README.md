# dd-agent-upgrade cookbook

Updates an installed Agent to the latest version (default), or the version
specified in `version`. You can also add a new repository by setting the
`add_new_repo` flag to `true` and passing an `aptrepo` and/or a `yumrepo`. This
is useful when you want to use the `dd-agent` recipe to install the latest
release, and then use this repository to add the candidate repository and
install the latest candidate.
