/*
    Bedrock Safeguard Inc. - The Gentlemen Ransomware Detection Rules
    https://github.com/Bedrock-Safeguard/gentlemen-decryptor
*/

rule Gentlemen_Ransomware_Binary
{
    meta:
        author = "Bedrock Safeguard Inc."
        description = "Detects The Gentlemen ransomware binary (Go/Garble)"
        date = "2026-04-20"
        yarahub_uuid = "2ec57bcd-b8a1-474a-85e2-33b3dbbdd069"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "4200b46a93c6ab059e2b34ce200c4a5b"
        reference = "https://github.com/Bedrock-Safeguard/gentlemen-decryptor"
        severity = "critical"

    strings:
        $footer_marker = "--marker--GENTLEMEN" ascii
        $eph_marker = "--eph--" ascii
        $tox_id = "88984846080D639C9A4EC394E53BA616D550B2B3AD691942EA2CCD33AA5B9340FD1A8FF40E9A" ascii
        $email = "negotiation_hapvida@proton.me" ascii
        $onion = "tezwsse5czllksjb7cwp65rvnk4oobmzti2znn42i43bjdfd2prqqkad.onion" ascii
        $locker_bg = "LOCKER_BACKGROUND" ascii
        $cynet_check = "Cynet Ransom Protection" ascii
        $gentlemen_bmp = "gentlemen.bmp" ascii
        $chacha_err1 = "chacha20: wrong HChaCha20 key size" ascii
        $chacha_err2 = "chacha20: wrong HChaCha20 nonce size" ascii
        $ecdh_err = "crypto/ecdh: invalid private key size" ascii
        $defender_excl = "Add-MpPreference -ExclusionProcess" ascii

    condition:
        uint16(0) == 0x5A4D and
        filesize < 10MB and
        (
            ($footer_marker and $eph_marker) or
            ($tox_id) or
            ($email and $onion) or
            (3 of ($locker_bg, $cynet_check, $gentlemen_bmp, $defender_excl)) or
            ($chacha_err1 and $chacha_err2 and $ecdh_err)
        )
}
