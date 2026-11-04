param(
    [ValidateSet('Start', 'Stop', 'Control', 'Plant', 'ClearConfig')][string]$Mode,
    [string]$Directory
)
$ErrorActionPreference = 'Stop'
$configRoot = 'C:\ProgramData\Datadog'
$site = 'config-trust.test:18443'
$hostsPath = "$env:SystemRoot\System32\drivers\etc\hosts"

switch ($Mode) {
    'Start' {
        Copy-Item $hostsPath "$Directory\hosts.original"
        Add-Content $hostsPath "`r`n127.0.0.1 api.config-trust.test instrumentation-telemetry-intake.config-trust.test"
        $cert = New-SelfSignedCertificate -DnsName 'api.config-trust.test', 'instrumentation-telemetry-intake.config-trust.test' -CertStoreLocation 'Cert:\LocalMachine\My' -KeyAlgorithm RSA -KeyLength 2048 -HashAlgorithm SHA256 -NotAfter (Get-Date).AddDays(1)
        Set-Content "$Directory\thumbprint" $cert.Thumbprint
        Export-Certificate -Cert $cert -FilePath "$Directory\receiver.cer" | Out-Null
        Import-Certificate -FilePath "$Directory\receiver.cer" -CertStoreLocation 'Cert:\LocalMachine\Root' | Out-Null
        & netsh http add sslcert ipport=0.0.0.0:18443 "certhash=$($cert.Thumbprint)" 'appid={B92FCE10-BC81-4A41-BA16-DF0D53F60492}' | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Failed to bind receiver certificate' }
        New-Item "$Directory\ssl-bound" -ItemType File | Out-Null
        New-Item "$Directory\requests.jsonl" -ItemType File | Out-Null
    }
    'Stop' {
        if (Test-Path "$Directory\ready") {
            Stop-Process -Id ([int](Get-Content "$Directory\ready")) -Force -PassThru -ErrorAction SilentlyContinue |
                Wait-Process -ErrorAction SilentlyContinue
        }
        if (Test-Path "$Directory\ssl-bound") {
            & netsh http delete sslcert ipport=0.0.0.0:18443 | Out-Null
        }
        if (Test-Path "$Directory\hosts.original") { Copy-Item "$Directory\hosts.original" $hostsPath -Force }
        if (Test-Path "$Directory\thumbprint") {
            $thumbprint = (Get-Content "$Directory\thumbprint").Trim()
            foreach ($store in @('My', 'Root')) {
                Remove-Item "Cert:\LocalMachine\$store\$thumbprint" -DeleteKey -ErrorAction SilentlyContinue
            }
        }
    }
    'Control' {
        # Do not disable certificate validation: this must prove trust and HTTPS reachability.
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        [Net.WebRequest]::DefaultWebProxy = $null
        Invoke-WebRequest -UseBasicParsing -Method Post -Uri "https://instrumentation-telemetry-intake.$site/control" -Body 'receiver-positive-control' | Out-Null
    }
    'Plant' {
        if (Test-Path $configRoot) { throw 'Config root must be absent before planting configuration' }
        New-Item -Path $configRoot -ItemType Directory | Out-Null
        [IO.File]::WriteAllText("$configRoot\datadog.yaml", "site: $site`n")
        $untrustedOwner = [Security.Principal.SecurityIdentifier]::new('S-1-5-32-545') # BUILTIN\Users
        $acl = Get-Acl $configRoot
        $acl.SetOwner($untrustedOwner)
        Set-Acl -Path $configRoot -AclObject $acl
        $owner = (Get-Acl $configRoot).GetOwner([Security.Principal.SecurityIdentifier]).Value
        if ($owner -ne $untrustedOwner.Value) { throw "Config root has unexpected fixture owner: $owner" }
    }
    'ClearConfig' {
        if (Test-Path $configRoot) {
            & takeown.exe /A /F $configRoot /R /D Y | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Failed to reclaim test configuration' }
            & icacls $configRoot /grant '*S-1-5-32-544:(OI)(CI)F' /T /C | Out-Null
            if ($LASTEXITCODE -ne 0) { throw 'Failed to grant cleanup access' }
            Remove-Item $configRoot -Recurse -Force
        }
    }
}
