rule PS_XOR_Reflection_Loader_StatusCode {
    meta:
        description = "Obfuscated PowerShell .NET loader (builder statusCode family): XOR byte decode + reflection + HMAC-32 + scriptblock/IEX exec"
        author      = "Marjoriefort"
        date        = "2026-09-13"
        reference   = "InTheWild.0438 / fresh powershell 2026-08-15 + tag_powershell"
        confidence  = "high"
        yarahub_uuid            = "fa18d2b4-4585-4d65-8880-c4b8f39df14e"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "064ccf9046b8382154ac1f3b203dfa51"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $err = /Error: statusCode [0-9]{5,6}/
        $xor = "$b[$i]=$b[$i] -bxor"
        $ref = "GetMethods()|Where-Object{$_.Name -eq $n"
        $iex = "'-Exp' + 'ress' + 'io' + 'n'"
    condition:
        $err and 1 of ($xor, $ref, $iex)
}
