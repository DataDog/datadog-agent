param (
    [string]$parameterName
)

$retryCount = 0
$maxRetries = 10

while ($retryCount -lt $maxRetries) {
    $result = (aws ssm get-parameter --region us-east-1 --name $parameterName --with-decryption --query "Parameter.Value" --output text 2> awsErrorFile.txt)
    $error = Get-Content awsErrorFile.txt
    if ($result) {
        $result
        break
    }
    if ($error -match "Unable to locate credentials") {
        # See 5th row in https://docs.google.com/spreadsheets/d/1JvdN0N-RdNEeOJKmW_ByjBsr726E3ZocCKU8QoYchAc
        Write-Error "Credentials won't be retrieved, no need to retry"
        exit 1
    }

    $retryCount++
    Start-Sleep -Seconds ([math]::Pow(2, $retryCount))
}
