rule win_imminentrat_j1_7e208e97 {

    meta:
        author                    = "Johannes Bader"
        date                      = "2021-10-01"
        description               = "detects the imminent rat"
        hash1                     = "a728603061b5aa98fa40fb0447ba71e3"
        hash2                     = "5d8446a23b80e9b6cb7406c2ba81d606685cf11b24e9eb8309153a47b04f3aad"
        malpedia_family           = "win.imminent_monitor_rat"
        tlp                       = "TLP:WHITE"
        version                   = "v1.0"
        yarahub_author_email      = "yara@bin.re"
        yarahub_author_twitter    = "@viql"
        yarahub_license           = "CC BY-SA 4.0"
        yarahub_reference_md5     = "a728603061b5aa98fa40fb0447ba71e3"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp  = "TLP:WHITE"
        yarahub_uuid              = "7e208e97-3295-4714-8797-6e0f56c7c354"

    strings:
        $str_mining_1 = "Downloading miner data" wide
        $str_mining_2 = "This client is already mining" wide
        $str_mining_3 = "Started mining successfully" wide
        $str_mining_4 = "Unable to start mining" wide
        $str_mining_5 = "-o {0} -u {1} -p {2} -a scrypt -I {3} -T {4}" wide

        $str_plugin_1 = "\\Imminent\\Plugins\\" wide

        $str_fingerprint_1 = "Screens: {0}" wide
        $str_fingerprint_2 = "Battery: {0}" wide
        $str_fingerprint_3 = "Ram Usage: {0}%" wide
        $str_fingerprint_4 = "Last Reboot: {0}" wide
        $str_fingerprint_5 = "Graphics Card: {0}" wide
        $str_fingerprint_6 = "Firewall: {0}" wide
        $str_fingerprint_7 = "Anti-Virus: {0}" wide
        $str_fingerprint_8 = "Unique Identifier: {0}" wide
        $str_fingerprint_9 = "Privileges: {0}" wide
        $str_fingerprint_10 = "MAC Address: {0}" wide
        $str_fingerprint_11 = "Client Location: {0}" wide
        $str_fingerprint_12 = "Ram: {0}" wide
        $str_fingerprint_13 = "LAN: {0}" wide
        $str_fingerprint_14 = "Processor: {0}" wide
        $str_fingerprint_15 = "Computer Username: {0}" wide
        $str_fingerprint_16 = "Operating System: {0}" wide
        $str_fingerprint_17 = "Client Identifier: {0}" wide
        $str_fingerprint_18 = "Computer Name: {0}" wide

        $str_filedownload_1 = "File downloaded & executed" wide
        $str_filedownload_2 = "File downloaded & updated" wide

    condition:
        uint16(0) == 0x5A4D and
        3 of ($str_mining_*) and
        $str_plugin_1 and
        15 of ($str_fingerprint_*) and
        all of ($str_filedownload_*)
}
