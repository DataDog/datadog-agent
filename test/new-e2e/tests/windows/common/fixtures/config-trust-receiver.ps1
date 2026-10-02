param([string]$Directory)
$ErrorActionPreference = 'Stop'
$listener = New-Object System.Net.HttpListener
$listener.Prefixes.Add('https://+:18443/')
try {
    $listener.Start()
    Set-Content -Path "$Directory\ready" -Value $PID
    while ($listener.IsListening) {
        $context = $listener.GetContext()
        $body = New-Object System.IO.MemoryStream
        $context.Request.InputStream.CopyTo($body)
        $record = @{
            path = $context.Request.Url.AbsolutePath
            apiKey = $context.Request.Headers['DD-Api-Key']
            body = [Convert]::ToBase64String($body.ToArray())
        } | ConvertTo-Json -Compress
        # Persist before acknowledging: once the installer exits, its completed submissions are observable.
        Add-Content -Path "$Directory\requests.jsonl" -Value $record -Encoding UTF8
        $body.Dispose()
        $context.Response.StatusCode = 200
        $context.Response.Close()
    }
} finally {
    $listener.Close()
}
