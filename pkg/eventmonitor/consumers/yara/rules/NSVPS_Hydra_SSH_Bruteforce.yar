rule NSVPS_Hydra_SSH_Bruteforce {
    meta:
        date = "2026-07-11"
        yarahub_uuid = "8fc5f28c-307a-4440-a75a-cfb14c7c0218"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d41d8cd98f00b204e9800998ecf8427e"
        description = "Hydra SSH brute-force campaign credentials pattern from NSVPS honeypot"
        author = "sanad (NSVPS-SOC)"
    strings:
        $u1 = "root" ascii
        $u2 = "admin" ascii
        $p1 = "123456" ascii
        $p2 = "password" ascii
        $hydra = "libssh" ascii
    condition:
        $hydra and 2 of ($u*, $p*)
}
