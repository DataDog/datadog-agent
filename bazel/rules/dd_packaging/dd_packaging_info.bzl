"""DdPackagingInfo — common provider for the DD packaging decorator chain."""

DdPackagingInfo = provider(
    "Common provider for automated dependencies packaging",
    fields = {
        "installed_files": "list of PackageFilegroupInfo to be installed alongside this target",
    },
)
