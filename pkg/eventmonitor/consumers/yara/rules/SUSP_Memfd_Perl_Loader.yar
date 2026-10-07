rule SUSP_Memfd_Perl_Loader
{
    meta:
        description = "Fileless memfd_create + argv[0]-spoofing Perl loader"
        author = "Peter"
        date = "2026-09-18"
        reference = "bincrypter hook / gsocket deploy.sh _config2bin_memexec()"
        yarahub_uuid = "28bee55e-26cc-4737-8785-0c9c3aa0e67b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9f81e196548efc844bd8c407a0be834c"
        reference_filename = "stage2.sh (decoded stage 2)"

    strings:
        $sysc   = "for(319,279,385,4314,4354)"
        $fd     = { 24 5E 46 3D 32 35 35 }                      // $^F=255
        $memfd  = { 73 79 73 63 61 6C 6C 24 5F 2C 24 22 2C }    // syscall$_,$",
        $procfd = { 2F 70 72 6F 63 2F 24 24 2F 66 64 2F }       // /proc/$$/fd/
        $exec   = { 65 78 65 63 7B 22 2F 70 72 6F 63 2F 24 24 } // exec{"/proc/$$

    condition:
        filesize < 10485760
        and $sysc
        and 2 of ($fd, $memfd, $procfd, $exec)
}
