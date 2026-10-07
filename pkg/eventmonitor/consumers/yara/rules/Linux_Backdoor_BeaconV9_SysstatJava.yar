rule Linux_Backdoor_BeaconV9_SysstatJava {
    meta:
        description = "Detects Beacon v9 Linux dropper leveraging JVM attachment, sysstat-masquerading relays, and systemd persistence"
        author = "Serhii Kocherhan"
        date = "2026-09-16"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "a949e6f1-6e73-4c0c-9bb3-30e3135c1b7f"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_link = "https://x.com/BlinkzSec/status/2100241251513536834"
        yarahub_reference_md5 = "4a292b499ed7a56bda441a45bb9e8c95"

    strings:
        // Actor Header & Configuration Markers
        $n1 = "beacon v9" ascii wide
        $n3 = "KILL_DATE=" ascii wide

        // Inject + Relay Deploy
        $n2_hex = { E6 B3 A8 E5 85 A5 20 2B 20 E4 B8 AD E7 BB A7 E9 83 A8 E7 BD B2 }

        // Dynamic C2 Endpoint Structure (Generalized IP & URI patterns)
        $c2_uri1 = "/bt" ascii wide
        $c2_uri2 = "/exec-out" ascii wide
        $c2_ip_regex = /R[0-9]?=http:\/\/[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}:[0-9]{2,5}/ ascii wide

        // IPC & Temporary Staging Artifacts
        $p1 = "/tmp/.kmod_in" ascii wide
        $p2 = "/tmp/.kmod_out" ascii wide
        $p3 = "/tmp/attach.class" ascii wide
        $p4 = "attach.class" ascii wide
        $p5 = "/tmp/agent" ascii wide

        // Persistence & Service Masquerading Paths
        $s1 = "/usr/local/bin/sysstat-sync" ascii wide
        $s2 = "sysstat-collect.service" ascii wide
        $s3 = "/usr/local/bin/.syscheck" ascii wide
        $s4 = "/usr/local/bin/.sysagent" ascii wide
        $s5 = "sys-jvm-monitor.path" ascii wide
        $s6 = "sys-jvm-monitor.service" ascii wide

        // Execution Mechanics & Behavior Fragments
        $b1 = "VirtualMachine.attach" ascii wide
        $b2 = "loadAgent" ascii wide
        $b3 = "touch -r" ascii wide
        $b4 = "systemctl enable" ascii wide
        $b5 = "#!/bin/bash" ascii wide

    condition:
        filesize < 512KB and
        $b5 at 0 and
        (
            // High Confidence: Direct actor tags or generic dynamic C2 assignment pattern
            $n1 or $n2_hex or
            ($n3 and $c2_ip_regex) or

            // Dynamic Endpoint & IPC Correlation
            (
                any of ($c2_uri*) and
                2 of ($p*) and
                2 of ($b1, $b2, $b3, $b4)
            ) or

            // Structural Dropper & Systemd Masquerading Combination
            (
                3 of ($p*) and
                3 of ($s*)
            ) or

            // Core Specific Path Set
            (
                $s1 and $s3 and $s5
            )
        )
}