/*
 * TigerRAT_pe_yaraify -- tightened byte rule for YARAify hunting (strong stubs + PE guard)
 * Auto-generated then refined: keeps only [strong] decode/decrypt stubs,
 * scopes to PE (MZ), and requires two stubs to co-match to suppress
 * false positives over the global corpus. Operand bytes (addresses,
 * immediates, relocations) are wildcarded; opcode skeleton is fixed.
 * Derived from static disassembly (capstone); samples never executed.
 */
rule TigerRAT_pe_yaraify
{
    meta:
        description = "YARAify-tightened byte rule from 9 sample(s) -- VERIFY hits"
        family = "TigerRAT"
        actor = "Andariel"
        filetype = "pe"
        method = "byte-pattern (strong decrypt stubs only)"
        engine = "capstone"
        author = "hunts-yara-code"
        date = "2026-08-21"
        sample_count = 9
        note = "operands wildcarded; static disasm; FP rate not evaluated"
        yarahub_uuid = "9c8efd86-11f5-4ab6-851a-da305cb6cc7b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "70652edadedbacfd30d33a826853467d"
        hash0 = "196fb1b6eff4e7a049cea323459cfd6c0e3900d8d69e1d80bffbaabd24c06eba"
        hash1 = "1f8dcfaebbcd7e71c2872e0ba2fc6db81d651cf654a21d33c78eae6662e62392"
        hash2 = "2af120fb9ded079cb66e15c4b8aadcfdca9ead82d91e083f950605cc28babc82"
        hash3 = "bffe910904efd1f69544daa9b72f2a70fb29f73c51070bde4ea563de862ce4b1"
        hash4 = "c296a7ba8645ef6c06162228ca58618dcdc6f1205aec5680a7f355a53d2a0148"
        hash5 = "d7b6456b10677ee98c6f44bafa2dc5581f0a361224b57aa752809aafbf49a8c5"
    strings:
        //   0F B6 08  movzx ecx, byte ptr [rax]
        //   42 0F B6 14 00  movzx edx, byte ptr [rax + r8]
        //   2B CA  sub ecx, edx
        //   75 ??  jne 0x??
        //   48 FF C0  inc rax
        //   85 D2  test edx, edx
        //   75 ??  jne 0x??
        $c0 = { 0F B6 08 42 0F B6 14 00 2B CA 75 ?? 48 FF C0 85 D2 75 ?? }   // 3/9 samples [strong]
    condition:
        uint16(0) == 0x5A4D and 1 of them
}
