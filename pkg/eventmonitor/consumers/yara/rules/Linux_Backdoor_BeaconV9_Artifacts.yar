rule Linux_Backdoor_BeaconV9_Artifacts {
    meta:
        description = "Detects Beacon v9 persistence and staging artifacts including sysstat masquerading, systemd units, and Java IPC paths"
        author = "Serhii Kocherhan"
        date = "2026-09-16"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "e02de9f0-baa4-4209-a4d1-78ea63d06e49"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://x.com/BlinkzSec/status/2100241251513536834"
        yarahub_reference_md5 = "4a292b499ed7a56bda441a45bb9e8c95"

    strings:
        // Systemd Persistence Artifacts
        $sysd_unit1 = "sys-jvm-monitor.service" ascii wide
        $sysd_unit2 = "sys-jvm-monitor.path" ascii wide

        // Binary & Script Masquerading Paths
        $bin_path1  = "/usr/local/bin/.syscheck" ascii wide
        $bin_path2  = "/usr/local/bin/.sysagent" ascii wide
        $bin_path3  = "/usr/local/bin/sysstat-sync" ascii wide

        // IPC & Temporary Staging Artifacts
        $stage_pipe1 = "/tmp/.kmod_in" ascii wide
        $stage_pipe2 = "/tmp/.kmod_out" ascii wide
        $stage_java  = "attach.class" ascii wide

        // C2 Infrastructure Indicators (Generalized IP and Specific Legacy IPs)
        $ip_regex    = /[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}:(8081|8888)/ ascii wide
        $c2_legacy1  = "95.217.82.39" ascii wide
        $c2_legacy2  = "51.38.168.29" ascii wide

    condition:
        filesize < 2MB and
        (
            // Broad artifact match threshold
            4 of ($sysd_unit*, $bin_path*, $stage_pipe*, $stage_java) or

            // Specific systemd path + masqueraded binary combination
            (any of ($sysd_unit*) and any of ($bin_path*)) or

            // Full Java IPC staging suite match
            ($stage_pipe1 and $stage_pipe2 and $stage_java) or

            // Network indicator correlation with staging or persistence paths
            (($ip_regex or any of ($c2_legacy*)) and 2 of ($bin_path*, $sysd_unit*, $stage_pipe*))
        )
}