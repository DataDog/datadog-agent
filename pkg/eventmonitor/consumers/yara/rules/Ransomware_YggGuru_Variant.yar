rule Ransomware_YggGuru_Variant
{
    meta:
        description = "Detects potential variants of YggGuru ransomware family"
        author = "anthony"
        date = "2026-03-09"
        malware_family = "YggGuru"
        malware_type = "ransomware"
        severity = 8
        yarahub_reference_md5 = "b61d829b6b1fc2ecc12a86db5d99d826"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = ""
        yarahub_uuid = "686eebef-25c2-4d85-8369-b76e0d83d892"
    strings:
        $c2 = "ygg.guru" ascii
        $go_runtime = "runtime.goroutine" ascii
        $go_main = "main.main" ascii
        $behav1 = "encryptFile" ascii
        $behav2 = "exfiltrateKey" ascii
        $behav3 = "deleteShadowCopies" ascii
        $behav4 = "dropRansomNote" ascii
        $behav5 = "killProcesses" ascii
        $behav6 = "addPersistence" ascii
        $behav7 = "disableTaskManager" ascii
        $behav8 = "createRansomWallpaper" ascii
        $behav9 = "enumerateDrives" ascii
        $proc1 = "outlook.exe" ascii
        $proc2 = "sqlservr.exe" ascii
        $proc3 = "mysqld.exe" ascii
        $proc4 = "oracle.exe" ascii
        $proc5 = "mongod.exe" ascii
        $proc6 = "postgres.exe" ascii
    condition:
        uint16(0) == 0x5A4D and filesize < 15MB and
        (
            ($c2 and 2 of ($behav*)) or
            ($go_runtime and $go_main and 5 of ($behav*)) or
            ($go_main and 3 of ($behav*) and 3 of ($proc*))
        )
}
