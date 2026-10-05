import "math"

rule HKTL_UPX_Packed_Magic_Zeroed
{
    meta:
        description = "UPX-packed ELF64 with the UPX! magics zeroed so upx -d refuses it"
        author = "Peter"
        date = "2026-09-18"
        reference = "https://github.com/hackerschoice/gsocket"
        yarahub_uuid = "301836be-db59-4422-9fa4-1bde8ec22f87"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "b9744f5ab86676b4a7f53cbb83655081"
        reference_filename = "payload.elf (UPX-packed ELF)"

    strings:
        $upx = "UPX!"

    condition:
        uint32(0) == 0x464c457f
        and uint8(4) == 2
        and filesize > 4096
        and filesize < 209715200
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38) + 4) == 0
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38)) != 0
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38) + 12) == 0
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38) + 16) > filesize
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38) + 20) > 0
        and ( uint8(uint32(0x20) + uint16(0x36) * uint16(0x38) + 32) == 2 or
              uint8(uint32(0x20) + uint16(0x36) * uint16(0x38) + 32) == 8 or
              uint8(uint32(0x20) + uint16(0x36) * uint16(0x38) + 32) == 14 )
        and uint8(uint32(0x20) + uint16(0x36) * uint16(0x38) + 33) == 0
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38) + 24) > 0
        and uint32(uint32(0x20) + uint16(0x36) * uint16(0x38) + 28) > 0
        and not $upx
        and math.entropy(0, filesize) > 7.0
}
