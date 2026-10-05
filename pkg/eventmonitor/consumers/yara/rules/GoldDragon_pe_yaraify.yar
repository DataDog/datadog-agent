/*
 * GoldDragon_pe_yaraify -- tightened byte rule for YARAify hunting (strong stubs + PE guard)
 * Auto-generated then refined: keeps only [strong] decode/decrypt stubs,
 * scopes to PE (MZ), and requires two stubs to co-match to suppress
 * false positives over the global corpus. Operand bytes (addresses,
 * immediates, relocations) are wildcarded; opcode skeleton is fixed.
 * Derived from static disassembly (capstone); samples never executed.
 */
rule GoldDragon_pe_yaraify
{
    meta:
        description = "YARAify-tightened byte rule from 7 sample(s) -- VERIFY hits"
        family = "GoldDragon"
        actor = "Kimsuky"
        filetype = "pe"
        method = "byte-pattern (strong decrypt stubs only)"
        engine = "capstone"
        author = "hunts-yara-code"
        date = "2026-08-21"
        sample_count = 7
        note = "operands wildcarded; static disasm; FP rate not evaluated"
        yarahub_uuid = "2d60f319-2b73-4c58-8e96-606b92bc3f3e"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "28833e121bb77c8262996af1f2aeef55"
        hash0 = "70298c1bfc6b8e07c0600f9264712211bcc7b57b28853d8143f249639cdf6569"
        hash1 = "a7a86cbf520c0ca37e2f8e37584fcd9c68e79614fd8352d10a7bb223c3a3a39b"
        hash2 = "4ff2a67b094bcc56df1aec016191465be4e7de348360fd307d1929dc9cbab39f"
        hash3 = "8f2cbc93b7cd5cdc54e1670105c3da682bae0b70bc6bc4b0c0c18ab5c40be9c4"
        hash4 = "97935fb0b5545a44e136ee07df38e9ad4f151c81f5753de4b59a92265ac14448"
        hash5 = "b1e28bc8720303326946ec69d8ad6c90b572e177d562bbe769abaf1aad3d9e1a"
    strings:
        //   0F B6 88 ?? ?? ?? ??  movzx ecx, byte ptr [eax + 0x??]
        //   66 83 BC 8E ?? ?? ?? ?? ??  cmp word ptr [esi + ecx*4 + 0x??], 0
        //   75 ??  jne 0x??
        //   0F B6 88 ?? ?? ?? ??  movzx ecx, byte ptr [eax + 0x??]
        //   66 83 BC 8E ?? ?? ?? ?? ??  cmp word ptr [esi + ecx*4 + 0x??], 0
        //   75 ??  jne 0x??
        //   0F B6 88 ?? ?? ?? ??  movzx ecx, byte ptr [eax + 0x??]
        //   66 83 BC 8E ?? ?? ?? ?? ??  cmp word ptr [esi + ecx*4 + 0x??], 0
        //   75 ??  jne 0x??
        //   0F B6 88 ?? ?? ?? ??  movzx ecx, byte ptr [eax + 0x??]
        //   66 83 BC 8E ?? ?? ?? ?? ??  cmp word ptr [esi + ecx*4 + 0x??], 0
        //   75 ??  jne 0x??
        //   83 E8 ??  sub eax, 4
        //   83 F8 ??  cmp eax, 3
        //   7D ??  jge 0x??
        $c0 = { 0F B6 88 ?? ?? ?? ?? 66 83 BC 8E ?? ?? ?? ?? ?? 75 ?? 0F B6 88 ?? ?? ?? ?? 66 83 BC 8E ?? ?? ?? ?? ?? 75 ?? 0F B6 88 ?? ?? ?? ?? 66 83 BC 8E ?? ?? ?? ?? ?? 75 ?? 0F B6 88 ?? ?? ?? ?? 66 83 BC 8E ?? ?? ?? ?? ?? 75 ?? 83 E8 ?? 83 F8 ?? 7D ?? }   // 5/7 samples [strong]
        //   83 FA ??  cmp edx, 2
        //   7D ??  jge 0x??
        //   42  inc edx
        //   8B CA  mov ecx, edx
        //   EB ??  jmp 0x??
        //   33 C9  xor ecx, ecx
        //   FF 86 ?? ?? ?? ??  inc dword ptr [esi + 0x??]
        //   8B 86 ?? ?? ?? ??  mov eax, dword ptr [esi + 0x??]
        //   89 8C 86 ?? ?? ?? ??  mov dword ptr [esi + eax*4 + 0x??], ecx
        //   8B 45 ??  mov eax, dword ptr [ebp - 0x??]
        //   66 89 3C 8B  mov word ptr [ebx + ecx*4], di
        //   C6 84 0E ?? ?? ?? ?? ??  mov byte ptr [esi + ecx + 0x??], 0
        //   FF 8E ?? ?? ?? ??  dec dword ptr [esi + 0x??]
        //   85 C0  test eax, eax
        //   74 ??  je 0x??
        //   0F B7 44 88 ??  movzx eax, word ptr [eax + ecx*4 + 2]
        //   29 86 ?? ?? ?? ??  sub dword ptr [esi + 0x??], eax
        //   83 BE ?? ?? ?? ?? ??  cmp dword ptr [esi + 0x??], 2
        //   7C ??  jl 0x??
        $c1 = { 83 FA ?? 7D ?? 42 8B CA EB ?? 33 C9 FF 86 ?? ?? ?? ?? 8B 86 ?? ?? ?? ?? 89 8C 86 ?? ?? ?? ?? 8B 45 ?? 66 89 3C 8B C6 84 0E ?? ?? ?? ?? ?? FF 8E ?? ?? ?? ?? 85 C0 74 ?? 0F B7 44 88 ?? 29 86 ?? ?? ?? ?? 83 BE ?? ?? ?? ?? ?? 7C ?? }   // 5/7 samples [strong]
    condition:
        uint16(0) == 0x5A4D and 2 of them
}
