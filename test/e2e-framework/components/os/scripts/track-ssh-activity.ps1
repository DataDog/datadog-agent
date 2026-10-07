# Keep the "last-activity" tag of this EC2 instance up to date while it is being used over SSH.
# Windows counterpart of track-ssh-activity.sh: test-infra-cleaner (aws-nuke) does not delete
# instances whose "last-activity" tag is recent, so instances that are still in use survive past
# their regular lifetime.
#
# Copied over SSH and run every 5 minutes by a SYSTEM scheduled task, see
# installWindowsSSHActivityTracking in scenarios/aws/ec2/os_win.go. There is no PAM on Windows to
# hook session opening, the periodic run alone is enough for the cleaner.
# Everything here is best effort: any failure just skips this run.

$ErrorActionPreference = 'Stop'

$tagKey = 'last-activity'
$imdsUrl = 'http://169.254.169.254/latest'

# Return $true if at least one SSH session is currently open on this host
function Test-SSHSessionActive {
  @(Get-NetTCPConnection -LocalPort 22 -State Established -ErrorAction SilentlyContinue).Count -gt 0
}

try {
  if (-not (Test-SSHSessionActive)) {
    exit 0
  }

  # PowerShell 5.1 may default to TLS 1.0, which the EC2 API endpoints reject
  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

  $imdsToken = (Invoke-WebRequest -UseBasicParsing -Method Put -TimeoutSec 2 -Uri "$imdsUrl/api/token" `
      -Headers @{ 'X-aws-ec2-metadata-token-ttl-seconds' = '300' }).Content
  function Get-Imds([string]$path) {
    (Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 -Uri "$imdsUrl/meta-data/$path" `
        -Headers @{ 'X-aws-ec2-metadata-token' = $imdsToken }).Content
  }

  $instanceId = Get-Imds 'instance-id'
  $region = Get-Imds 'placement/region'
  $role = Get-Imds 'iam/security-credentials/'
  $creds = Get-Imds "iam/security-credentials/$role" | ConvertFrom-Json

  # Sign an EC2 CreateTags call with SigV4 using only .NET, so that we do not depend on
  # AWS Tools for PowerShell being installed on the AMI.
  function Get-Sha256Hex([string]$data) {
    $hash = [Security.Cryptography.SHA256]::Create().ComputeHash([Text.Encoding]::UTF8.GetBytes($data))
    ([BitConverter]::ToString($hash) -replace '-', '').ToLowerInvariant()
  }
  function Get-HmacSha256([byte[]]$key, [string]$data) {
    [Security.Cryptography.HMACSHA256]::new($key).ComputeHash([Text.Encoding]::UTF8.GetBytes($data))
  }

  $invariant = [Globalization.CultureInfo]::InvariantCulture
  $now = [DateTime]::UtcNow
  $tagValue = $now.ToString("yyyy-MM-dd'T'HH:mm:ss'Z'", $invariant)
  $amzDate = $now.ToString("yyyyMMdd'T'HHmmss'Z'", $invariant)
  $dateStamp = $now.ToString('yyyyMMdd', $invariant)
  $endpoint = "ec2.$region.amazonaws.com"
  $contentType = 'application/x-www-form-urlencoded; charset=utf-8'
  $body = "Action=CreateTags&ResourceId.1=$instanceId&Tag.1.Key=$tagKey&Tag.1.Value=$([Uri]::EscapeDataString($tagValue))&Version=2016-11-15"

  $signedHeaders = 'content-type;host;x-amz-date;x-amz-security-token'
  $canonicalRequest = "POST`n/`n`ncontent-type:$contentType`nhost:$endpoint`nx-amz-date:$amzDate`nx-amz-security-token:$($creds.Token)`n`n$signedHeaders`n$(Get-Sha256Hex $body)"
  $scope = "$dateStamp/$region/ec2/aws4_request"
  $stringToSign = "AWS4-HMAC-SHA256`n$amzDate`n$scope`n$(Get-Sha256Hex $canonicalRequest)"

  $kDate = Get-HmacSha256 ([Text.Encoding]::UTF8.GetBytes("AWS4$($creds.SecretAccessKey)")) $dateStamp
  $kRegion = Get-HmacSha256 $kDate $region
  $kService = Get-HmacSha256 $kRegion 'ec2'
  $kSigning = Get-HmacSha256 $kService 'aws4_request'
  $signature = ([BitConverter]::ToString((Get-HmacSha256 $kSigning $stringToSign)) -replace '-', '').ToLowerInvariant()

  Invoke-WebRequest -UseBasicParsing -Method Post -TimeoutSec 10 -Uri "https://$endpoint/" `
    -ContentType $contentType -Body $body -Headers @{
    'X-Amz-Date'           = $amzDate
    'X-Amz-Security-Token' = $creds.Token
    'Authorization'        = "AWS4-HMAC-SHA256 Credential=$($creds.AccessKeyId)/$scope, SignedHeaders=$signedHeaders, Signature=$signature"
  } | Out-Null
} catch {
  exit 0
}
