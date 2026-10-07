rule Whitelock_AESGCM_KeySetup_Stub_x64_v1 {
  meta:
    author                      = "Valton Tahiri (cybee.ai)"
    description                 = "AES-GCM key and nonce copy stub in 64-bit PE (rep movs patterns)"
    date                        = "2025-10-10"
    tlp                         = "TLP:white"
    yarahub_license             = "CC0 1.0"
    yarahub_rule_matching_tlp  = "TLP:WHITE"
    yarahub_rule_sharing_tlp   = "TLP:WHITE"
    yarahub_uuid                = "4f41f51d-4b0e-44c6-9fa4-b9ae6bb3d8c2"
    yarahub_reference_md5      = "9e35477130cd2731755a35e8b4c0429b" 

  strings:
    $stub_a = { 48 81 EC ?? 01 00 00 [0-64] B9 20 00 00 00 [0-16] F3 48 A5 [0-64] B9 03 00 00 00 [0-16] F3 48 A5 }
    $stub_b = { 48 81 EC ?? 01 00 00 [0-64] B9 20 00 00 00 [0-16] F3 48 A5 [0-64] B9 0C 00 00 00 [0-16] F3 A4 }

  condition:
    uint16(0) == 0x5A4D and
    uint32(uint32(0x3C)) == 0x00004550 and
    uint16(uint32(0x3C) + 4) == 0x8664 and
    filesize < 52428800 and
    ( $stub_a or $stub_b )
}