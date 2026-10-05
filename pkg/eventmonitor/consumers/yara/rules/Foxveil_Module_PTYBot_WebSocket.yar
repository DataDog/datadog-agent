rule Foxveil_Module_PTYBot_WebSocket
{
    meta:
        yarahub_uuid = "3c1b0a7e-5d64-4f2a-9b31-8e0d7a6c41f5"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "cb27f99f797344e9fe6cd55d34f6bee5"
        description = "Foxveil second-stage macOS module: ad-hoc-signed fat x86_64+arm64 C++ remote-shell bot. Same internal runtime as the clipper module (packer's empty randomly-named SG_PROTECTED_VERSION_1 segment once per slice, compile-time ANSI-C-LCG stack-string obfuscator 0x41c64e6d/0x3039), plus the bot-specific capability anchors: the full System V pseudo-terminal quartet, the SHA-1 initial state in little-endian byte order used for the RFC 6455 Sec-WebSocket-Accept handshake, and the contiguous BOT_DEV_MODE/dev_user development switch left in __cstring"
        actor = "Foxveil"
        family = "Foxveil bot module"
        reference_sha256 = "08074ec033c1a5dcfb0125a352f7f499f29f61d8ed2ce2f693d48813074810d4"
        date = "2026-09-22"
        tlp = "CLEAR"
        confidence = "high"

    strings:
        // FAT_CIGAM, nfat_arch = 2, first cputype x86_64.
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }

        // Only two dylibs: everything else is resolved at runtime through dlsym.
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"

        // Foxveil packer artefact, once per slice: LC_SEGMENT_64 (0x19), cmdsize 0x48 (no
        // sections), segname "__" + junk, vmaddr at 0x1_08xx_xxxx above __LINKEDIT, then
        // vmsize / fileoff / filesize / maxprot / initprot / nsects all zero and
        // flags = SG_PROTECTED_VERSION_1 (8). Same shape as Foxveil_Loader_EmptySeg_Variant.
        $emptyseg = { 19 00 00 00 48 00 00 00 5F 5F [14] 00 ?? ?? 08 01 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 08 00 00 00 }

        // Shared module runtime: every literal is built on the stack from a compile-time
        // ANSI-C LCG (state = state * 0x41c64e6d + 0x3039), so the multiplier is materialised
        // once per protected literal.
        $lcg_x64 = { 6D 4E C6 41 }
        $lcg_a64 = { ?? CD 89 52 ?? 38 A8 72 }

        // System V pseudo-terminal quartet in the import table: the module allocates a PTY and
        // runs an interactive shell on it. Dropping any of these removes the terminal feature.
        $pty1 = "_posix_openpt"
        $pty2 = "_grantpt"
        $pty3 = "_unlockpt"
        $pty4 = "_ptsname"

        // SHA-1 initial state stored as little-endian bytes (67452301 EFCDAB89 98BADCFE
        // 10325476 C3D2E1F0). Required by the RFC 6455 Sec-WebSocket-Accept handshake, so it
        // cannot be rotated while the module keeps its WebSocket transport.
        $sha1iv = { 01 23 45 67 89 AB CD EF FE DC BA 98 76 54 32 10 F0 E1 D2 C3 }

        // Development switch left in __cstring, contiguous, once per slice.
        $botdev  = "BOT_DEV_MODE\x00dev_user\x00"
        $botdev2 = "BOT_DEV_MODE"

    condition:
        $macho_fat at 0 and filesize > 131072 and filesize < 8388608 and
        $dylib1 and $dylib2 and
        #emptyseg >= 2 and
        (#lcg_x64 >= 20 or #lcg_a64 >= 20) and
        all of ($pty*) and
        #sha1iv >= 2 and
        ($botdev or $botdev2)
}
