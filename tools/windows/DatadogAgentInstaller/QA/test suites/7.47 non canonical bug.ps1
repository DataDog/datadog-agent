using Module "..\test_framework.psm1"

Suite "7.47 non-canonical bug upgrade test suite" {
    BeforeTest {
        $installResult = Install-MSI -installerName ddagent-cli-7.44.1.msi
        Assert_InstallSuccess $installResult

        Assert_DaclAutoInherited_Flag_Set "C:\"
        Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog"
        Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog\Datadog Agent"
    }
    
    # This test case demonstrates the bug
    Case "Upgrade 7.44.0 to 7.47.0 with rollback" {
        Require @(
            "ddagent-cli-7.44.1.msi",
            "datadog-agent-7.47.0-1.x86_64.msi"
        )
        Test {
            $installResult = Install-MSI -installerName datadog-agent-7.47.0-1.x86_64.msi -msiArgs "WIXFAILWHENDEFERRED=1"
            Assert_InstallFinishedWithCode $installResult 1603

            Assert_DaclAutoInherited_Flag_NotSet "C:\"
            # Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog"
            # Sometimes this is set, sometimes not :/
            Check_DaclAutoInherited_Flag "C:\Program Files\Datadog"
            Assert_DaclAutoInherited_Flag_NotSet "C:\Program Files\Datadog\Datadog Agent"
        }
    }

    # This test case demonstrates the bug fixed in 7.50
    Case "Upgrade 7.44.0 to 7.50.0 with rollback" {
        Require @(
            "ddagent-cli-7.44.1.msi",
            "datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi"
        )
        Test {
            # First rollback causes the AI flag to be removed
            $installResult = Install-MSI -installerName datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi -msiArgs "WIXFAILWHENDEFERRED=1"
            Assert_InstallFinishedWithCode $installResult 1603

            # But a successful install puts it back
            $installResult = Install-MSI -installerName datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi
            Assert_InstallSuccess $installResult

            Assert_DaclAutoInherited_Flag_Set "C:\"
            Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog"
            Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog\Datadog Agent"
        }
    }
}

Suite "7.47 non-canonical bug new installs test suite" {
    Case "New 7.50 install with uninstall" {
        Require @("datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi")
        Test {
            $installResult = Install-MSI -installerName datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi
            Assert_InstallSuccess $installResult
    
            Assert_DaclAutoInherited_Flag_Set "C:\"
            Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog"
            Assert_DaclAutoInherited_Flag_Set "C:\Program Files\Datadog\Datadog Agent"

            $installResult = Uninstall-DatadogAgent
            Assert_InstallSuccess $installResult
            Assert_DaclAutoInherited_Flag_Set "C:\"
            Assert_DirectoryNotExists "C:\Program Files\Datadog"
            Assert_DirectoryNotExists "C:\Program Files\Datadog\Datadog Agent"
        }
    }
    
    Case "New 7.50 install with rollback" {
        Require @("datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi")
        Test {
            $installResult = Install-MSI -installerName datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi -msiArgs "WIXFAILWHENDEFERRED=1"
            Assert_InstallFinishedWithCode $installResult 1603
    
            Assert_DaclAutoInherited_Flag_Set "C:\"
            Assert_DirectoryNotExists "C:\Program Files\Datadog"
            Assert_DirectoryNotExists "C:\Program Files\Datadog\Datadog Agent"
        }
    }
    
    Case "New 7.50 install in a different directory" {
        Require @("datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi")
        Test {
            $installResult = Install-MSI -installerName datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi -msiArgs "PROJECTLOCATION=C:\datadog APPLICATIONDATADIRECTORY=C:\datadog_data"
            Assert_InstallSuccess $installResult
    
            Assert_DaclAutoInherited_Flag_Set "C:\"
    
            Assert_DirectoryNotExists "C:\Program Files\Datadog"
            Assert_DirectoryNotExists "C:\Program Files\Datadog\Datadog Agent"
    
            Assert_DaclAutoInherited_Flag_Set "C:\datadog"
            Assert_DaclAutoInherited_Flag_Set "C:\datadog_data"
        }
    }

    Case "New 7.50 install in a different directory with rollback" {
        Require @("datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi")
        Test {
            $installResult = Install-MSI -installerName datadog-agent-ng-7.49.0-devel.git.532.2e75f76-1-x86_64.msi -msiArgs "PROJECTLOCATION=C:\datadog APPLICATIONDATADIRECTORY=C:\datadog_data WIXFAILWHENDEFERRED=1"
            Assert_InstallFinishedWithCode $installResult 1603

            Assert_DaclAutoInherited_Flag_Set "C:\"

            # Those folders are not removed on rollback, same for the OG installer
            # Assert_DirectoryNotExists "C:\datadog"
            # Assert_DirectoryNotExists "C:\datadog_data"
        }
    }
}
