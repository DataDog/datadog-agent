rule ELF_Packer_Dropper_Fileless_Armhf {
    meta:
        description = "Detects obfuscated Linux ELF packers/droppers featuring ChaCha20/RC4 decryption, memfd_create/execveat fileless execution capabilities, and unique binary artifacts."
        author = "Serhii Kocherhan"
        date = "2026-09-21"
        yarahub_twitter = "@skocherhan"
        yarahub_uuid = "b3ee06cf-4b52-43c1-a25a-a51289aa6469"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "47e06d34af0aea9b87207f86d7e87b67"

    strings:
        // Unique Strings and Error Messages
        $s_putita = "putita" ascii wide
        $s_linker_err = "/lib/ld-linux-armhf.so.3: No such file or directory" ascii wide
        
        // Cryptographic Constants (ChaCha20)
        $s_chacha = "expand 32-byte k" ascii wide

        // Temporary Writable Directories & Fileless Execution Artifacts
        $p_shm = "/dev/shm" ascii wide
        $p_var_run = "/var/run" ascii wide
        $p_var_tmp = "/var/tmp" ascii wide
        $s_proc_fd = "/proc/self/fd/" ascii wide

    condition:
        // Validate ELF Header (\x7F ELF)
        uint32(0) == 0x464C457F and
        filesize < 20MB and
        (
            $s_putita or
            $s_linker_err or
            ($s_chacha and ($s_proc_fd or any of ($p_*)))
        )
}