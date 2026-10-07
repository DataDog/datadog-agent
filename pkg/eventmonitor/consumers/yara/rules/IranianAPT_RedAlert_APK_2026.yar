rule IranianAPT_RedAlert_APK_2026 {
    meta:
        description     = "RedAlert fake APK spyware family - Operation Epic Fury (Iran/Israel 2026)"
        author          = "Paolo Costanzo - paolocostanzo.github.io"
        date            = "2026-03-15"
        reference       = "https://paolocostanzo.github.io/operation-epic-fury-cyber-war-iran/"
        hash_redalert   = "83651b0589665b112687f0858bfe2832ca317ba75e700c91ac34025ee6578b72"
        hash_umgdn      = "0cba66e78ddaeecfdd462c8cb39e443d083dc58c609b0edc73e8101e59ca91e8"
        mitre_attack    = "T1437, T1430, T1636.002"
        confidence      = "HIGH"
        yarahub_uuid              = "2164d6a5-416c-4d65-ae79-bc7f3f2f39ba"
        yarahub_license           = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_reference_md5     = "9c6c67344fecd8ff8dbbee877aad7efc"

    strings:

        $pkg_stage1     = "com.red.alertx" ascii
        $pkg_stage2     = "com.net.alerts" ascii


        $c2_host        = "api.ra-backup.com" ascii
        $c2_path        = "/analytics/submit.php" ascii


        $pushy_sdk      = "me.pushy.sdk" ascii
        $pushy_api      = "api.pushy.me" ascii


        $spoof_store    = "com.android.vending" ascii
        $dex_debug      = "DebugProbesKt.dex" ascii

    condition:
        uint32(0) == 0x04034b50
        and (
            ($pkg_stage1 and $c2_host)
            or ($pkg_stage2 and $c2_host)
            or ($c2_path and ($pushy_sdk or $pushy_api))
            or ($pkg_stage1 and $spoof_store and $dex_debug)
        )
}
