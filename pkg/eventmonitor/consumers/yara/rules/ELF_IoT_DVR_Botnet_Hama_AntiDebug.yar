rule ELF_IoT_DVR_Botnet_Hama_AntiDebug {
    meta:
        description = "Detects IoT/DVR botnet malware targeting ELF binaries with ptrace anti-debugging on ARM/MIPS"
        author = "Serhii Kocherhan"
        date = "2026-08-21"
        yarahub_uuid = "5a01ebad-46a4-43c1-8c0b-7dd0bd5203e3"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // ELF Header Magic Bytes
        $elf_magic = { 7F 45 4C 46 }

        // Exploitation & Persistence Strings
        $exploit_uri = "/device.rsp?opt=sys&cmd=___S_O_S_T_R_E_A_MAX___" ascii wide
        $path_hama   = "/var/tmp/.hama" ascii
        $path_h      = "/var/tmp/.h" ascii
        $proc_exe    = "/proc/%d/exe" ascii

        // --- ptrace(PTRACE_TRACEME, 0, 0, 0) Byte Sequences ---

        // ARM 32-bit (ARM Mode, Little Endian)
        // MOV R0, #0 ; MOV R7, #26 (0x1A - sys_ptrace) ; SVC 0 / SWI 0
        $ptrace_arm_le = { 00 00 a0 e3 (1a 70 a0 e3 | 1a 70 87 e2) 00 00 00 ef }

        // ARM Thumb Mode (Little Endian)
        // MOVS R0, #0 ; MOVS R7, #26 ; SVC 0
        $ptrace_thumb_le = { 00 20 1a 27 00 df }

        // ARM64 / AArch64 (Little Endian)
        // MOV X0, #0 ; MOV X8, #117 (0x75 - sys_ptrace) ; SVC #0
        $ptrace_arm64_le = { 00 00 80 d2 a8 0e 80 d2 01 00 00 d4 }

        // MIPS32 Big Endian
        // LI $a0, 0 ; LI $v0, 4026 (0x0FAA - sys_ptrace) ; SYSCALL
        $ptrace_mips_be = { 24 04 00 00 24 02 0f aa 00 00 00 0c }

        // MIPS32 Little Endian
        // LI $a0, 0 ; LI $v0, 4026 (0x0FAA - sys_ptrace) ; SYSCALL
        $ptrace_mips_le = { 00 00 04 24 aa 0f 02 24 0c 00 00 00 }

    condition:
        // Validate ELF Header at offset 0 and restrict max binary size to 5MB
        $elf_magic at 0 and filesize < 5MB and
        (
            $exploit_uri or
            (
                1 of ($path_hama, $path_h, $proc_exe) and
                any of ($ptrace_arm_le, $ptrace_thumb_le, $ptrace_arm64_le, $ptrace_mips_be, $ptrace_mips_le)
            )
        )
}