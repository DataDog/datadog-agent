rule LIN_Downloader_Botnet_KernelNames_MultiArch
{
    meta:
        description = "Downloader botnet ELF multi-archi - binaires deguises en taches kernel Linux, C2 IP en clair"
        author = "Marjoriefort"
        yarahub_reference_md5 = "609de44ce240cb413e1ec558dc637851"
        date = "2026-09-19"

        yarahub_uuid = "608fc3a7-ad45-4e38-92eb-ebfab884429b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://github.com/Marjoriefort/yara-rules"
    strings:
        $srv = /http:\/\/[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\/bins/
        $k1 = "kworker_u8"
        $k2 = "ksoftirqd0"
        $k3 = "bioset0"
        $k4 = "kblockd0"
        $k5 = "jbd2_sda1d"
        $k6 = "rcuop_0"
        $k7 = "ecryptfsd"
        $k8 = "kswapd0"
        $k9 = "xfsaild_sda"
        $u = "uname -m"

    condition:
        $u and (2 of ($k*) or ($srv and 1 of ($k*))) and filesize < 20KB
}
