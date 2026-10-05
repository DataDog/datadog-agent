rule ELF_IoT_DVR_Botnet_Hama {
    meta:
        description = "Detects IoT/DVR botnet malware using /device.rsp exploit, /var/tmp/.hama persistence, anti-analysis, and process killing"
        author = "Serhii Kocherhan"
        date = "2026-08-19"
        yarahub_uuid = "b594ff5f-9a79-4d43-a37a-c6b674c26ca6"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "8908d6f6452c1789abe4fda1fe9d01ca"

    strings:
        // Exploitation & Scanning Signatures
        $exploit_uri = "/device.rsp?opt=sys&cmd=___S_O_S_T_R_E_A_MAX___" ascii wide
        $scanner_func = "scanner_thread" ascii fullword

        // Persistence & File Paths
        $path_hama = "/var/tmp/.hama" ascii
        $path_h    = "/var/tmp/.h" ascii
        $cron_reboot = "@reboot" ascii
        $rcs_path  = "/etc/init.d/rcS" ascii

        // Process Killing & Memory Inspection
        $proc_exe  = "/proc/%d/exe" ascii
        $proc_maps = "/proc/%d/maps" ascii
        $killer_1  = "killer_exe" ascii fullword
        $killer_2  = "killer_maps" ascii fullword

        // Anti-Analysis & System Reconnaissance
        $sys_dmi   = "/sys/class/dmi/id/" ascii
        $proc_cpu  = "/proc/cpuinfo" ascii

    condition:
        // Primary anchor: The unique DVR exploit URI string
        $exploit_uri or
        (
            // Secondary match across key behavioral capabilities
            1 of ($path_hama, $path_h) and
            1 of ($proc_exe, $proc_maps, $killer_1, $killer_2) and
            1 of ($sys_dmi, $proc_cpu) and
            any of ($cron_reboot, $rcs_path, $scanner_func)
        )
}