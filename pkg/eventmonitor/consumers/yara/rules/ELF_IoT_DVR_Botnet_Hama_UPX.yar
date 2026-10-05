rule ELF_IoT_DVR_Botnet_Hama_UPX {
    meta:
        description = "Detects packed and unpacked ELF IoT/DVR botnet variants targeting UPX compression structures and uncompressed code"
        author = "Serhii Kocherhan"
        date = "2026-08-21"
        yarahub_uuid = "44afa12b-7ee2-44cd-8826-0f92c542ec84"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "00000000000000000000000000000000"

    strings:
        // ELF Header Magic Bytes
        $elf_magic = { 7F 45 4C 46 }

        // --- UPX Structural Signatures ---
        // "UPX!" Magic Header (Present in standard UPX headers/stubs)
        $upx_magic = "UPX!" ascii
        // Common UPX Section Names in ELF header tables
        $upx_sec0  = "UPX0" ascii
        $upx_sec1  = "UPX1" ascii
        $upx_sec2  = "UPX!" ascii fullword

        // --- Original Payload Strings (Unpacked Variants) ---
        $exploit_uri = "/device.rsp?opt=sys&cmd=___S_O_S_T_R_E_A_MAX___" ascii wide
        $path_hama   = "/var/tmp/.hama" ascii
        $proc_exe    = "/proc/%d/exe" ascii

        // --- Modified UPX Stub Signatures (Modified/Stripped UPX Headers) ---
        // Decompressor stub instructions common to UPX-packed ELF binaries on ARM
        $upx_arm_stub = { 04 F0 2D E5 00 10 A0 E3 (00 20 A0 E3 | 01 20 A0 E3) }

    condition:
        // Must be an ELF binary under 5MB
        $elf_magic at 0 and filesize < 5MB and
        (
            // MATCH 1: Unpacked variants matching payload strings
            ($exploit_uri or ($path_hama and $proc_exe))
            or
            // MATCH 2: Standard UPX Packed variants (Requires UPX magic + section markers)
            (
                $upx_magic and 
                any of ($upx_sec*)
            )
            or
            // MATCH 3: Modified UPX Packed variants (UPX headers erased/patched by threat actors)
            (
                $upx_arm_stub and
                filesize < 500KB
            )
        )
}