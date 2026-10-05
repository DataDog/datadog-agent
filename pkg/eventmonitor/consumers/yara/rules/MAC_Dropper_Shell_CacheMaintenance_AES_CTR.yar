rule MAC_Dropper_Shell_CacheMaintenance_AES_CTR
{
    meta:
        description = "macOS fake cache-maintenance dropper - payload AES-128-CTR via openssl, noms de commandes fragmentes"
        author = "Marjoriefort"
        yarahub_reference_md5 = "a6ba15c7cf3243d71fe6fe3e14e4f740"
        date = "2026-09-19"
        family = "macos-dropper-cachemaintenance"

        yarahub_uuid = "8f362e43-92fb-4307-b27c-34050fca1dc7"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $a = "_cache_root=\"$HOME/Library/Caches\""
        $b = "_rollout_key="
        $c = "enc -d -aes-128-ctr"
        $d = "-iv 00000000000000000000000000000000"
        $e = "eval \"$_r\""

    condition:
        3 of them and filesize < 30KB
}
