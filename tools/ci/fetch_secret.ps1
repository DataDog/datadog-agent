param (
    [string]$parameterName,
    [string]$parameterField,
    [string]$tempFile
)

$retryCount = 0
$maxRetries = 4

# To catch the error message from aws cli
$ErrorActionPreference = "Continue"

while ($retryCount -lt $maxRetries) {
    if ($parameterField) {
        $result = (vault kv get -field="$parameterField" kv/k8s/gitlab-runner/datadog-agent/"$parameterName" 2> errorFile.txt)
    } else {
        $result = (aws ssm get-parameter --region us-east-1 --name $parameterName --with-decryption --query "Parameter.Value" --output text 2> errorFile.txt)
    }
    $error = Get-Content errorFile.txt
    if ($result) {
        "$result" | Out-File -NoNewline -FilePath "$tempFile" -Encoding ASCII
        exit 0
    } else {
        Write-Error "$error"
    }
    if ($error -match "Unable to locate credentials") {
        # This error needs a restart of the job
        Write-Error "Permanent error: unable to locate credentials, not retrying"
        exit 42
    } 
    $retryCount++
    Start-Sleep -Seconds ([math]::Pow(2, $retryCount))
}

Write-Error "Failed to retrieve $parameterName after $maxRetries retries"
exit 1
