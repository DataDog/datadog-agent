rule LIN_Downloader_Unknown_WgetChmodExecute {
    meta:
        description = "Script shell downloader : wget+curl, chmod 777, execution, effacement (rm -rf) - botnet style"
        author      = "Marjoriefort"
        date        = "2026-09-17"
        reference   = "misses_archive / grappe 5.182.210.174"
        confidence  = "high"
        yarahub_uuid            = "e9c377ca-515f-4ea6-9b31-d0408bc7c17d"
        yarahub_license           = "CC0 1.0"
        yarahub_reference_md5   = "6bb737a05a57a0faad9cab3e7b10fadf"
        yarahub_reference_link    = "https://github.com/Marjoriefort/yara-rules"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
    strings:
        $a = "wget http://" ascii wide
        $b = "curl -O http://" ascii wide
        $c = "chmod 777" ascii wide
        $d = "rm -rf" ascii wide
    condition:
        $c and $d and 1 of ($a, $b)
}
