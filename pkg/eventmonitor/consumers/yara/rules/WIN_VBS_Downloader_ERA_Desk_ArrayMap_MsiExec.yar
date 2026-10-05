rule WIN_VBS_Downloader_ERA_Desk_ArrayMap_MsiExec
{
    meta:
        description = "VBS downloader genere par ERA DESK - obfuscation array-map, elevation UAC, MSI silencieux depuis le cloud"
        author = "Marjoriefort"
        yarahub_reference_md5 = "27b11776dc5f81f56f653695019f4fb0"
        date = "2026-09-19"

        yarahub_uuid = "1228c860-38a1-4cd0-9ea2-2f247377a5b3"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "ERA DESK"
        $b = "WScript.Arguments.Named.Exists(\"elev\")"
        $c = "\"ru\" & \"nas\""
        $d = "msiexec.exe"
        $e = "/quiet /norestart"
        $f = ".r2.dev/"

    condition:
        $a and 2 of ($b, $c, $d, $e, $f) and filesize < 30KB
}
