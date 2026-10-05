rule Gibberdrop_AMOS_ConfigRolXor
{
    meta:
        yarahub_uuid = "6bba38ab-fefb-4c20-85c7-08f8e9be66e4"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b6000eb45cc980f5fba975a5ccc9f909"
        description = "AMOS macOS stealer (mainline panel builds) - rolling-key XOR decrypt loop of the embedded __data config (key rotated right by 1 per byte), arm64 and x86_64"
        actor = "Gibberdrop"
        family = "AMOS"
        reference_sha256 = "fd0cd32edfe9b4f68dcc2fddb8f27c6102e9a9c8b90c5def07ac8e62b2a75303"
        date = "2026-09-27"
        tlp = "CLEAR"

    strings:
        // arm64: bfxil x11,x10,#0,#3 ; ldrb w11,[x11] ; add x12,x22,x10 ; ldrb w13,[x12,#0x10] ;
        //        eor w11,w13,w11 ; strb w11,[x12,#0x10] ; ror x9,x9,#1 ; str x9,[sp,#..]
        $a64 = { 4B 09 40 B3 6B 01 40 39 CC 02 0A 8B 8D 41 40 39 AB 01 0B 4A 8B 41 00 39 29 05 C9 93 }
        // x86_64: and edi,7 ; mov dil,[rbp+rdi-..] ; xor [rcx+rsi+0x10],dil ; ror rax,1 ; mov [rbp-..],rax ; inc rcx
        $x64 = { 83 E7 07 40 8A 7C 3D ?? 40 30 7C 31 10 48 D1 C8 48 89 45 ?? 48 FF C1 }

    condition:
        (uint32be(0) == 0xCAFEBABE or uint32(0) == 0xFEEDFACF) and filesize < 2MB and any of them
}
