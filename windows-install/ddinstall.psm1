function HasValidDDSignature($asset) {
    if (-not (Test-Path $asset -PathType Leaf)) {
        throw [System.IO.FileNotFoundException] "$asset not found"
    }

    $signature = Get-AuthenticodeSignature $asset

    if ($signature.Status -ne "Valid")
    {
        return $false
    }

    if ($signature.SignerCertificate.Subject -ne 'CN="Datadog, Inc", O="Datadog, Inc", L=New York, S=New York, C=US')
    {
        return $false
    }

    if ($signature.SignerCertificate.Issuer -ne 'CN=DigiCert Trusted G4 Code Signing RSA4096 SHA384 2021 CA1, O="DigiCert, Inc.", C=US')
    {
        return $false
    }

    return $true
}

function InstallAgentWithDotnetTracer($apiKey, $datadogSite) {
    Invoke-WebRequest -Uri https://s3.amazonaws.com/ddagent-windows-stable/datadog-agent-7-latest.amd64.msi -OutFile datadog-agent.msi
    if (-not (HasValidDDSignature("datadog-agent.msi")))
    {
        throw [System.IO.FileNotFoundException] "datadog-agent.msi did not pass the signature check."
    }

    Invoke-WebRequest -Uri https://github.com/DataDog/dd-trace-dotnet/releases/download/v2.47.0/datadog-dotnet-apm-2.47.0-x64.msi -OutFile datadog-dotnet-apm.msi
    if (-not (HasValidDDSignature("datadog-dotnet-apm.msi")))
    {
        throw [System.IO.FileNotFoundException] "datadog-dotnet-apm.msi did not pass the signature check."
    }

    Start-Process -Wait msiexec -ArgumentList '/qn /i datadog-agent.msi APIKEY=$apiKey SITE=$datadogSite'
    Start-Process -Wait msiexec -ArgumentList '/qn /i datadog-dotnet-apm.msi'
}
