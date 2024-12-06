using Module "..\test_framework.psm1"

Suite "benchmark-install" {
    BeforeTest {
        Disable-NetAdapter -Name "*" -Confirm:$false
    }

    Case "7.60.0 main" {
        Require @(
            "datadog-agent-7.60.0-devel.git.12.73a3f63.pipeline.45962468-1-x86_64.msi"
        )
        Test {
            Write-Host ("{0}s" -f (Measure-Command { Assert_InstallSuccess (Install-MSI -installerName datadog-agent-7.60.0-devel.git.12.73a3f63.pipeline.45962468-1-x86_64.msi )}).TotalSeconds)
        }
    }

    Case "7.60.0 7zr" {
        Require @(
            "datadog-agent-7.60.0-devel.git.16.c10553f.pipeline.45963056-1-x86_64.msi"
        )
        Test {
            Write-Host ("{0}s" -f (Measure-Command { Assert_InstallSuccess (Install-MSI -installerName datadog-agent-7.60.0-devel.git.16.c10553f.pipeline.45963056-1-x86_64.msi )}).TotalSeconds)
        }
    }
}

Suite "benchmark-upgrade" {
    BeforeTest {
        Disable-NetAdapter -Name "*" -Confirm:$false
        Assert_InstallSuccess (Install-MSI -installerName ddagent-cli-7.46.0.msi)
    }

    Case "Upgrade from 7.46.0 OG to 7.60.0 main" {
        Require @(
            "ddagent-cli-7.46.0.msi",
            "datadog-agent-7.60.0-devel.git.12.73a3f63.pipeline.45962468-1-x86_64.msi"
        )
        Test {
            Write-Host ("{0}s" -f (Measure-Command {
                Assert_InstallSuccess (Install-MSI -installerName datadog-agent-7.60.0-devel.git.12.73a3f63.pipeline.45962468-1-x86_64.msi)
            }).TotalSeconds)
        }
    }

    Case "Upgrade from 7.46.0 OG to 7.60.0 7zr" {
        Require @(
            "ddagent-cli-7.46.0.msi",
            "datadog-agent-7.60.0-devel.git.16.c10553f.pipeline.45963056-1-x86_64.msi"
        )
        Test {
            Write-Host ("{0}s" -f (Measure-Command {
                Assert_InstallSuccess (Install-MSI -installerName datadog-agent-7.60.0-devel.git.16.c10553f.pipeline.45963056-1-x86_64.msi)
            }).TotalSeconds)
        }
    }
}