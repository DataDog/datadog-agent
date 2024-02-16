param (
    [string]$parameterName
)

$retryCount = 0
$maxRetries = 5

if ($env:TraceLevel -eq '1') {
    $script:originalDebugPreference = $DebugPreference
    $DebugPreference = 'SilentlyContinue'
    $script:enableDebug = { $DebugPreference = 'Continue' }
    Register-EngineEvent PowerShell.Exiting -Action $enableDebug -SupportEvent
}

while ($retryCount -lt $maxRetries) {
    $result = (aws ssm get-parameter --region us-east-1 --name $parameterName --with-decryption --query "Parameter.Value" --output text)

    if ($result) {
        $result
        break
    }

    $retryCount++
    Start-Sleep -Seconds ([math]::Pow(2, $retryCount))
}
