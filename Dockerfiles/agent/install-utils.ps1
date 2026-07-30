function Invoke-WebRequestWithRetry {
    $MaxAttempts = 5
    for ($i = 1; $i -le $MaxAttempts; $i++) {
        try {
            Invoke-WebRequest @args
            return
        } catch {
            if ($i -ge $MaxAttempts) {
                throw
            }
            Write-Host ("Attempt #{0} failed: {1}" -f $i, $_)
            Start-Sleep -Seconds ($i * $i)
        }
    }
}
