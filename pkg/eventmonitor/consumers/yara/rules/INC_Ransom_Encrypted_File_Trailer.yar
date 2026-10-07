rule INC_Ransom_Encrypted_File_Trailer
{
    meta:
        description                = "Detects a file encrypted by the INC Ransom Linux/ESXi encryptor via its 83-byte trailer; extension independent, so it also catches symlink-encrypted files that kept their original name and files from an interrupted run"
        author                     = "Peter Gabaldon"
        date                       = "2026-09-18"
        yarahub_uuid               = "283da70d-8d4b-4a77-8f06-de6079d9e9be"
        yarahub_license            = "CC0 1.0"
        yarahub_rule_matching_tlp  = "TLP:WHITE"
        yarahub_rule_sharing_tlp   = "TLP:WHITE"
        yarahub_reference_md5      = "ae7652e2f2c2f8061747ead7473983fc"

    condition:
        // trailer layout, little-endian:
        //   -83 eph_pubkey[32] | -51 sha256(eph_pubkey)[32] | -19 cipher u32
        //   -15 block_size u32 | -11 skip u32 | -7 chunks u32 | -3 "INC"
        filesize >= 83 and
        uint8( filesize - 3 ) == 0x49 and                  // 'I'
        uint8( filesize - 2 ) == 0x4E and                  // 'N'
        uint8( filesize - 1 ) == 0x43 and                  // 'C'
        uint32( filesize - 19 ) <= 1 and                   // cipher: 0 = AES-128-CTR, 1 = Salsa20
        uint32( filesize - 15 ) > 0 and                    // block size
        uint32( filesize - 15 ) <= 0x4000000 and
        uint32( filesize - 11 ) <= 64                      // skip: 12 fast / 4 medium / 2 slow
}
