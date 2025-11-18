"""A module for bringing in transitive dependencies of rules_jvm_external"""

load("@rules_jvm_external//:defs.bzl", "maven_install")

def deps_jvm_external():
    maven_install(
        artifacts = [
            "com.android.support.constraint:constraint-layout:aar:1.1.2",
            "com.android.support:appcompat-v7:aar:26.1.0",
        ],
        repositories = [
            "https://jcenter.bintray.com/",
            "https://maven.google.com",
            "https://repo1.maven.org/maven2",
        ],
    )
