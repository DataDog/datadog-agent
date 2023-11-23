# Code to help define the tests
# This code will execute in the context of the host machine

class TestContext {
    [string]$VMName
    [PSCredential]$VMCredentials
    [string]$VMInitialSnapshotName
    [string]$QaSessionName
    [string]$QaArtifactsPath

    TestContext([string]$vmName, [string]$vmUserName, [string]$vmUserPassword, [string]$vmInitialSnapshotName, [string]$qaSessionname, [string]$qaArtifactsPath) {
        $this.VMName = $vmName
        $this.VMCredentials  = New-Object System.Management.Automation.PSCredential($vmUserName, (ConvertTo-SecureString $vmUserPassword -AsPlainText -Force))
        $this.VMInitialSnapshotName = $vmInitialSnapshotName
        $this.QaSessionName = $qaSessionname
        $this.QaArtifactsPath = $qaArtifactsPath
    }

    [void]Prepare_Host_Machine() {
        if ((Get-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-Management-PowerShell).State -ne "Enabled") {
            Write-Host "Hyper-V Management Module for PowerShell missing, activating it"
            Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-Management-PowerShell
        }
    }

    [void]Prepare_Target_Machine() {
        if ((Get-VMCheckpoint -VMName $this.VMName -Name $this.QaSessionName -ErrorAction SilentlyContinue) -eq $null) {
            # No snapshot for our test exists
            Restore-VMSnapshot -Name $this.VMInitialSnapshotName -VMName $this.VMName -Confirm:$false
            Checkpoint-VM -Name $this.VMName -SnapshotName $this.QaSessionName -Confirm:$false -ErrorAction Stop
        }
    }
}

class TestCase {
    [string]$TestDescription
    [scriptblock]$TestCode
    [string[]]$Artifacts

    TestCase([string]$testDescription, [string[]]$artifacts, [scriptblock]$testCode) {
        $this.TestDescription = $testDescription;
        $this.Artifacts = $artifacts;
        $this.TestCode = $testCode;
    }
}

# Collection of test cases
class TestSuite {
    [string]$SuiteDescription
    [TestCase[]]$TestCases
    [scriptblock]$BeforeTest
    [scriptblock] hidden $CommonCode

    TestSuite([string]$suiteDescription, [scriptblock]$beforeTest, [TestCase[]]$testCases) {
        $this.SuiteDescription = $suiteDescription
        $this.BeforeTest = $beforeTest
        $this.TestCases = $testCases
        $this.CommonCode = Get-Command $PSScriptRoot\common_test_code.ps1 | Select-Object -ExpandProperty ScriptBlock 
    }

    [void]Execute([TestContext]$context) {
        $this.Execute($context, $false)
    }

    [void]Execute([TestContext]$context, [bool]$resetSnapshot) {
    
        if ($resetSnapshot -eq $true) {
            Remove-VMSnapshot -VMName $context.VMName -Name "$($context.QaSessionName)_$($this.SuiteDescription)"  -Confirm:$false -ErrorAction SilentlyContinue
        }

        if ((Get-VMCheckpoint -VMName $context.VMName -Name "$($context.QaSessionName)_$($this.SuiteDescription)" -ErrorAction SilentlyContinue) -eq $null) {
            Restore-VMSnapshot -Name $context.QaSessionName -VMName $context.VMName -Confirm:$false
            # Create the base check point for all the tests in this suite
            $artifactsForTests = @()
            ($this.TestCases) | ForEach-Object {
                $_.Artifacts | ForEach-Object {
                    $artifactsForTests += $_
                }
            }
            $artifactsForTests | Sort-Object -Unique | ForEach-Object {
                Copy-VMFile $context.VMName -SourcePath "$($context.QaArtifactsPath)\$($_)" -DestinationPath "C:\$($_)" -CreateFullPath -FileSource Host -ErrorAction Stop
            }
            Invoke-Command -Credential $context.VMCredentials -VMName $context.VMName -ScriptBlock ([scriptblock]::Create([string]($this.commonCode) + "`n" + [string]($this.BeforeTest)))
            Checkpoint-VM "$($context.QaSessionName)_$($this.SuiteDescription)" -VMName $context.VMName -Confirm:$false
        }

        $this.TestCases | ForEach-Object {
            Restore-VMSnapshot -Name "$($context.QaSessionName)_$($this.SuiteDescription)" -VMName $context.VMName -Confirm:$false
            # $beforeTestScript has to be a string otherwise it doesn't capture the TestDescription value
            $beforeTestScript = "report_info `"Running scenario '" + $_.TestDescription  + "'`""
            $finalScript = [scriptblock]::Create([string]($this.commonCode) + "`n" + [string]$beforeTestScript + "`n" + [string]($_.TestCode))
            Invoke-Command -Credential $context.VMCredentials -VMName $context.VMName -ScriptBlock $finalScript
        }
    }
}

# Helper methods to make test definitions pretty
$global:testSuites = @()
$global:currentSuite = $null
function Suite($desc, $script) {
    $global:currentSuite = [TestSuite]::new($desc, {}, @())
    Invoke-Command $script
    $global:testSuites += $global:currentSuite
}

function BeforeTest($script) {
    $global:currentSuite.BeforeTest = $script
}

$global:currentCase = $null
function Case($desc, $script) {
    $global:currentCase = [TestCase]::new($desc, @(), {})
    $global:currentSuite.TestCases += $currentCase
    Invoke-Command $script
}

function Require([string[]]$artifacts) {
    $global:currentCase.Artifacts = $artifacts
}

function Test($script) {
    $global:currentCase.TestCode = $script
}
