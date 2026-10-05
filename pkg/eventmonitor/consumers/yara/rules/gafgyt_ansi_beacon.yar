rule gafgyt_ansi_beacon
{
    meta:
        description = "Detects Gafgyt variant with custom ANSI-colored IP beacon"
        author = "Liho"
        family = "Gafgyt"
        reference = "Custom bot variant using ANSI red in IP report string"
        date = "2025-05-23"
        yarahub_uuid = "3539ba8e-3438-4746-958a-9fbeda2b647b"
        yarahub_license = "CC0 1.0"
        yarahub_rule_matching_tlp = "TLP:WHITE"
        yarahub_rule_sharing_tlp = "TLP:WHITE"
        yarahub_reference_md5 = "2b93409dc9370b647fbc56a3ff32aa3f"

    strings:
        $beacon = { 25 73 20 1B 5B 31 3B 33 31 6D 69 70 3A 25 73 }

    condition:
        $beacon
}
