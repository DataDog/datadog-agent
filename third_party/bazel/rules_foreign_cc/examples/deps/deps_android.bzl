"""A module for bringing in transitive dependencies of rules_android"""

load("@rules_android//android:rules.bzl", "android_ndk_repository", "android_sdk_repository")

def deps_android():
    android_sdk_repository(
        name = "androidsdk",
    )

    android_ndk_repository(
        name = "androidndk",
    )
