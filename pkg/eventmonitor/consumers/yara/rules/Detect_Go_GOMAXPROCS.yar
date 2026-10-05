rule Detect_Go_GOMAXPROCS
{
    meta:
        author                       = "Obscurity Labs LLC"
        description                  = "Detects Go binaries by the presence of runtime.GOMAXPROCS in the runtime metadata"
        version                      = "1.0.0"
        date                         = "2025-06-05"
        yarahub_author_twitter       = "@obscuritylabs"
        yarahub_reference_md5        = "7ff72f21d83d3abdc706781fb3224111"
        yarahub_uuid                 = "90626ac0-e544-4d5b-b8c3-e70a7feb2b68"
        yarahub_license              = "CC BY-NC 4.0"
        yarahub_rule_matching_tlp    = "TLP:WHITE"
        yarahub_rule_sharing_tlp     = "TLP:WHITE"

    strings:
        $gomax = "runtime.GOMAXPROCS" ascii

    condition:
        $gomax
}
