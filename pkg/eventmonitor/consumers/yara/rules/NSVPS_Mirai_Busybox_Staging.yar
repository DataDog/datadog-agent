rule NSVPS_Mirai_Busybox_Staging {
    meta:
        date = "2026-07-11"
        yarahub_uuid = "e873fadf-89d5-4a8d-9fbb-29aced75256b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d41d8cd98f00b204e9800998ecf8427e"
        description = "Mirai-style busybox wget/tftp payload staging seen in NSVPS honeypot"
        author = "sanad (NSVPS-SOC)"
    strings:
        $b1 = "/bin/busybox" ascii
        $w1 = "wget http://" ascii
        $t1 = "tftp -g" ascii
        $chmod = "chmod 777" ascii
    condition:
        $b1 and ($w1 or $t1) and $chmod
}
