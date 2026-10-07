rule HKTL_GsNetcat_Stealth_Build
{
    meta:
        description = "gs-netcat stealth build in packed or unpacked form"
        reference = "https://github.com/hackerschoice/gsocket/tree/beta"
        author = "Peter Gabaldon"
        date = "2026-08-23"
        severity = "high"
        note = "Dual-use: the first branch matches an unpacked ELF; the second matches the packed configured ELF"
        yarahub_reference_link = "https://bazaar.abuse.ch/sample/89e906327d8e85067e16f3eb077a4a891fd01773460363b235918035314703ea/"
        yarahub_reference_md5 = "b9744f5ab86676b4a7f53cbb83655081"
        yarahub_uuid = "a1d4924f-9018-431a-a015-cddde53b7b48"
        yarahub_license = "CC BY 4.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"

    strings:
        // Unpacked ELF indicators.
        $c1 = "GS_CONFIG_SECRET='%s'"
        $c2 = "GS_CONFIG_PROC_HIDDENNAME='%s'"
        $c3 = "GSOCKET_CONFIG_CHECK"
        $c4 = "GS_CONFIG_MEMEXEC=1"
        $c5 = "GSOCKET_PROC_HIDDENNAME"

        $s1 = "gs.thc.org"
        $s2 = "-stealth"
        $s3 = "_GS_REEXEC_DONE"
        $s4 = "_GS_SYSTEMD_RUN=1 exec systemd-run --quiet --scope --user"

        // Packed configured ELF indicators.
        $config_capable = "8xKd12TX"
        $config_on_disk = { 8C CC FF D0 85 86 E0 EC }

    condition:
        uint32(0) == 0x464c457f
        and (
            // Unpacked ELF branch.
            ( 3 of ($c*) and 1 of ($s*) )
            or
            // Packed configured stealth-build branch.
            ( $s2 and $config_capable and $config_on_disk )
        )
}
