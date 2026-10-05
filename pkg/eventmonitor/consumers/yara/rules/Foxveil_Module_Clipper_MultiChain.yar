rule Foxveil_Module_Clipper_MultiChain
{
    meta:
        yarahub_uuid = "76e92ee9-0c1c-42a8-afda-322bf72c6a63"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "9fde93ed3b267513a1f21509cda529fe"
        description = "Foxveil second-stage macOS module: ad-hoc-signed fat x86_64+arm64 C++ multi-chain cryptocurrency address clipper. Anchors on the contiguous chain-prefix table (bc1q/bc1p/ltc1/addr1/stake1/bitcoincash:/bnb1/cosmos1/X-avax1/P-avax1/DdzFF) next to the bech32 and base58 alphabets, and on the compile-time LCG string obfuscator (ANSI-C rand constants 0x41c64e6d/0x3039) that builds every literal on the stack"
        actor = "Foxveil"
        family = "Foxveil clipper module"
        reference_sha256 = "a7c0d024ed24dcbc1b17cbda430a39edb52325c30ad9b7207c0267e43fbec4b7"
        date = "2026-09-22"
        tlp = "CLEAR"
        confidence = "high"

    strings:
        // FAT_CIGAM, nfat_arch = 2, first cputype x86_64. Literal, no module calls.
        $macho_fat = { CA FE BA BE 00 00 00 02 01 00 00 07 }

        // The only two dylibs: pure C++, everything else resolved at runtime via dlopen/dlsym.
        $dylib1 = "/usr/lib/libSystem.B.dylib\x00"
        $dylib2 = "/usr/lib/libc++.1.dylib\x00"

        // __TEXT,__cstring: the address-family prefix table, emitted contiguously by the
        // compiler in this exact order, once per slice. This is the clipper's chain
        // recogniser and cannot be rotated without rewriting the address parser.
        $chains = "bc1q\x00bc1p\x00ltc1\x00addr1\x00stake1\x00bitcoincash:\x00q\x00p\x00bnb1\x00cosmos1\x00X-avax1\x00P-avax1\x00DdzFF\x00"

        // Alphabets sitting beside it: bech32/bech32m and Bitcoin base58.
        $bech32 = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"
        $b58    = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

        // Compile-time string obfuscator. Every literal in the binary is built on the stack as
        //     state = state * 0x41c64e6d + 0x3039 ; stack[i] = (state ^ imm_i) & 0xff
        // so the multiplier is materialised once per protected literal, hundreds of times.
        // x86-64: the multiplier as a 32-bit instruction immediate.
        $lcg_x64 = { 6D 4E C6 41 }
        // arm64: MOVZ Wd,#0x4e6d followed immediately by MOVK Wd,#0x41c6,LSL#16.
        $lcg_a64 = { ?? CD 89 52 ?? 38 A8 72 }

        // Foxveil packer artefact, once per slice: LC_SEGMENT_64 (0x19), cmdsize 0x48 (no
        // sections), segname "__" + junk, vmaddr at 0x1_08xx_xxxx above __LINKEDIT, then
        // vmsize / fileoff / filesize / maxprot / initprot / nsects all zero and
        // flags = SG_PROTECTED_VERSION_1 (8). Same shape as Foxveil_Loader_EmptySeg_Variant.
        $emptyseg = { 19 00 00 00 48 00 00 00 5F 5F [14] 00 ?? ?? 08 01 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00
                      00 00 00 00 08 00 00 00 }

    condition:
        $macho_fat at 0 and filesize > 131072 and filesize < 8388608 and
        $dylib1 and $dylib2 and
        #chains >= 2 and $bech32 and $b58 and
        (#lcg_x64 >= 8 or #lcg_a64 >= 8) and
        #emptyseg >= 2
}
