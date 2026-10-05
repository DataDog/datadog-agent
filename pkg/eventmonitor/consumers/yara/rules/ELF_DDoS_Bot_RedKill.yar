rule ELF_DDoS_Bot_RedKill {
    meta:
        description = "Detects RedKill Linux DDoS botnet ELF executables based on persistence file paths, bot identifiers, systemd service details, and network artifacts."
        author = "Serhii Kocherhan"
        date = "2026-09-19"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "5e864da3-2afa-48dc-977b-c00ef1ca73ce"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "760b9cf0893e11b27522330a604385f6"

    strings:
        // Bot Identity and API Strings
        $s_bot = "RedKill" ascii wide
        $s_domain = "api.snapcraft.io" ascii wide nocase

        // Persistence Paths & Service Name
        $p_service = "redkill.service" ascii wide
        $p_tmp1 = "/tmp/.redkill" ascii wide
        $p_tmp2 = "/var/tmp/.redkill" ascii wide
        $p_dev = "/dev/.redkill" ascii wide

        // Shell & System Config Targets
        $c_rc = "/etc/rc.local" ascii wide
        $c_bashrc1 = "/etc/bash.bashrc" ascii wide
        $c_bashrc2 = "/.bashrc" ascii wide

    condition:
        // Validate ELF Binary Header (\x7F ELF)
        uint32(0) == 0x464C457F and
        filesize < 10MB and
        
        // Ensure core bot identity is present along with persistence/network indicators
        $s_bot and $s_domain and (
            2 of ($p_*) or
            2 of ($c_*)
        )
}