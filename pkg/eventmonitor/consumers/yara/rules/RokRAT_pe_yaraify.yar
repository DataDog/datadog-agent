/*
 * RokRAT_pe_yaraify -- tightened byte rule for YARAify hunting (strong stubs + PE guard)
 * Auto-generated then refined: keeps only [strong] decode/decrypt stubs,
 * scopes to PE (MZ), and requires two stubs to co-match to suppress
 * false positives over the global corpus. Operand bytes (addresses,
 * immediates, relocations) are wildcarded; opcode skeleton is fixed.
 * Derived from static disassembly (capstone); samples never executed.
 */
rule RokRAT_pe_yaraify
{
    meta:
        description = "YARAify-tightened byte rule from 7 sample(s) -- VERIFY hits"
        family = "RokRAT"
        actor = "APT37"
        filetype = "pe"
        method = "byte-pattern (strong decrypt stubs only)"
        engine = "capstone"
        author = "hunts-yara-code"
        date = "2026-08-21"
        sample_count = 7
        note = "operands wildcarded; static disasm; FP rate not evaluated"
        yarahub_uuid = "9250c135-edb2-40d8-8eac-408c1000ff23"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "d5fe744b9623a0cc7f0ef6464c5530da"
        hash0 = "3be58a7a7a25dbceee9e7ef06ef20aa86aef083be19db9e5ffb181d3f9f6615a"
        hash1 = "41d9b6d8cf0fff85bf35327d4b94db629cd9f754c487672911b7f701fe8c5539"
        hash2 = "5ca7f6603eb01705ec76307ca6c64f694a4f2132c84413a0751520b8a3961716"
        hash3 = "7d514021c472e6e17f587ed30555d3f120653e6c7f8dc25d2331514b92ffd7bc"
        hash4 = "aa76b4db29cf929b4b22457ccb8cd77308191f091cde2f69e578ade9708d7949"
        hash5 = "af619936fa29b7d0cf0c8441674bbf062cea427f9aaad4ea3173b5942956720b"
    strings:
        //   8B 13  mov edx, dword ptr [rbx]
        //   8D 5B ??  lea ebx, [rbx]
        //   33 C0  xor eax, eax
        //   03 55 ??  add edx, dword ptr [rbp]
        //   66 0F 1F 44 00 ??  nop word ptr [rax + rax]
        //   0F BE 0A  movsx ecx, byte ptr [rdx]
        //   8D 52 ??  lea edx, [rdx]
        //   C1 C8 ??  ror eax, 0
        //   03 C1  add eax, ecx
        //   80 7A ?? ??  cmp byte ptr [rdx], 0
        //   75 ??  jne 0x??
        //   03 C6  add eax, esi
        //   3B 45 ??  cmp eax, dword ptr [rbp]
        //   74 ??  je 0x??
        //   8B 45 ??  mov eax, dword ptr [rbp]
        //   47 3B 78 ??  cmp r15d, dword ptr [r8]
        //   72 ??  jb 0x??
        $c0 = { 8B 13 8D 5B ?? 33 C0 03 55 ?? 66 0F 1F 44 00 ?? 0F BE 0A 8D 52 ?? C1 C8 ?? 03 C1 80 7A ?? ?? 75 ?? 03 C6 3B 45 ?? 74 ?? 8B 45 ?? 47 3B 78 ?? 72 ?? }   // 3/7 samples [strong]
        //   8A 04 0B  mov al, byte ptr [rbx + rcx]
        //   C1 CE ??  ror esi, 0
        //   3C ??  cmp al, 0
        //   0F BE C0  movsx eax, al
        //   7C ??  jl 0x??
        //   83 C6 ??  add esi, 0
        //   41 03 F0  add esi, r8d
        //   3B CA  cmp ecx, edx
        //   72 ??  jb 0x??
        $c1 = { 8A 04 0B C1 CE ?? 3C ?? 0F BE C0 7C ?? 83 C6 ?? 41 03 F0 3B CA 72 ?? }   // 3/7 samples [strong]
        //   0F BE 0A  movsx ecx, byte ptr [rdx]
        //   8D 52 ??  lea edx, [rdx]
        //   C1 C8 ??  ror eax, 0
        //   03 C1  add eax, ecx
        //   80 7A ?? ??  cmp byte ptr [rdx], 0
        //   75 ??  jne 0x??
        $c2 = { 0F BE 0A 8D 52 ?? C1 C8 ?? 03 C1 80 7A ?? ?? 75 ?? }   // 3/7 samples [strong]
    condition:
        uint16(0) == 0x5A4D and 2 of them
}
