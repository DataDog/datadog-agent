rule Ransomware_YggGuru_Unpacked
{
    meta:
        description = "Detects YggGuru ransomware (unpacked)"
        author = "anthony"
        date = "2026-03-09"
        hash = "c3b71a279800e79535b096b110c9cf27ff16b92cf0a18200dd777a9abae9a5e7"
        malware_family = "YggGuru"
        malware_type = "ransomware"
        severity = 10
        yarahub_reference_md5 = "b61d829b6b1fc2ecc12a86db5d99d826"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_author_twitter = ""
        yarahub_uuid = "e9d923ec-72a3-4fd1-a0f1-d76f2f232070"
    strings:
        $c2_domain = "ygg.guru" ascii
        $c2_endpoint = "ygg.guru/api/health" ascii
        $c2_email = "noreply@ygg.guru" ascii
        $btc_wallet = "bc1q9x4m27d3kge0f8v5td0w4rjhkfz5a5nqpfsghr" ascii
        $note1 = "ALL YOUR FILES HAVE BEEN ENCRYPTED" ascii
        $note2 = "Do NOT rename or modify encrypted files" ascii
        $note3 = "Payment increases to 1 BTC after 72 hours" ascii
        $func_exfil = "main.exfiltrateKey" ascii
        $func_shadow = "main.deleteShadowCopies" ascii
        $func_encrypt = "main.encryptFile" ascii
        $func_ransomnote = "main.dropRansomNote" ascii
        $func_wallpaper = "main.createRansomWallpaper" ascii
        $func_persist = "main.addPersistence" ascii
        $func_taskmgr = "main.disableTaskManager" ascii
        $func_kill = "main.killProcesses" ascii
        $module = "poc-cyber/cmd/ransomware" ascii
        $crypto1 = "pocrypto.Encrypt" ascii
        $crypto2 = "pocrypto.GenerateKey" ascii
        $exfil_struct = "exfilPayload" ascii
    condition:
        uint16(0) == 0x5A4D and filesize < 10MB and
        (
            (any of ($c2_*) and any of ($note*)) or
            $module or
            (4 of ($func_*)) or
            ($crypto1 and $exfil_struct and any of ($note*)) or
            ($btc_wallet and any of ($note*)) or
            ($crypto1 and $crypto2 and any of ($func_*))
        )
}
