rule SCRIPT_Sample_Unique_b82d28bb {
    meta:
        description = "Specimen unique (soumission Bazaar) - strings distinctifs propres au sample"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "b82d28bbdc0012d2ca01005b9d152ad6b53c29c0f6dd9a3896f8c32086b3870d.bat"
        confidence  = "low"
        note        = "Regle specimen : vise la re-soumission identique. Verifier en sandbox."
        yarahub_uuid              = "0bb4361f-5df8-4986-91e0-0a2fb0166029"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5     = "00f1ede49e8ae76db97ee19e3a05652b"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "powershell -Command \"$ErrorActionPreference='Stop'; $ProgressPreference='SilentlyContinue'; [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12; $client = New-Object System.Net.WebClient; $client.DownloadFile('%DOWNLOAD_URL%', '%MSI_FILE%')\" >nul 2>&1" ascii wide
        $b = "    netsh advfirewall firewall add rule name=\"ScreenConnect Server Port\" dir=out action=allow protocol=TCP localport=%SERVER_PORT% remoteport=%SERVER_PORT% enable=yes profile=any >nul 2>&1" ascii wide
    condition:
        all of them
}
