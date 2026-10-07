rule LNK_plugx_mustang_panda
{
    meta:
        description = "Detects LNK files with embedded obfuscated PowerShell scripts that extract and execute embedded payloads"
        author = "marsomx"
        date = "2026-01-12"
        yarahub_uuid = "8290afe3-2bdf-44fe-87e3-9788fddd2958"
        reference = "LNK dropper with embedded archive extraction"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "7CA528C170164F9945C87D5BA673B7B0"

    strings:
        // LNK file header
        $lnk_header = { 4C 00 00 00 01 14 02 00 }

        // PowerShell invocation patterns (case-insensitive)
        $ps1 = "powershell" nocase
        $ps2 = "powershell.exe" nocase

        // File operations - obfuscated or normal
        $fileop1 = "Get-ChildItem" nocase wide
        $fileop2 = "System.IO.File" nocase wide
        $fileop4 = "OpenRead" nocase wide
        $fileop5 = "New-Object byte[]" nocase wide

        // Archive/extraction operations
        $archive1 = /\.zip/ nocase wide
        $archive2 = /\.tar/ nocase wide
        $archive3 = /\.gz/ nocase wide

        $tar = /tar\s+\-[xv]+/ nocase wide

        // Environment variable access patterns
        $env1 = "$Env:appdata" nocase wide
        $env2 = "$Env:temp" nocase wide
        $env3 = "$Env:tmp" nocase wide
        $env4 = "$Home" nocase wide
        $env5 = /\$Env:\w+/ nocase wide

        // Sleep/delay operations
        $sleep1 = "Sleep -Seconds" nocase wide


    condition:
         // Must be a LNK file
        $lnk_header at 0 and

        // Must have PowerShell execution
        1 of ($ps*) and

        // Must have file operations
        all of ($fileop*) and

        // Must have environment variable access
        2 of ($env*) and

        1 of ($archive*) and $sleep1 and $tar
}