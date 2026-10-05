rule dependsonpythonailib {
  meta:
    author = "Tim Brown"
    yarahub_author_twitter = "@timb_machine"
    description = "Hunts for dependencies on Python AI libraries"
    date = "2025-05-10"
    yarahub_reference_md5	= "b0275236f4d75d4825e4d0f02bc89064"
    yarahub_uuid = "e06804d6-635e-44d9-9b32-6829e38a9990"
    yarahub_license = "CC BY 4.0"
    yarahub_rule_matching_tlp = "TLP:WHITE"
    yarahub_rule_sharing_tlp = "TLP:WHITE"
  strings:
    $torch = "torch"
    $tensorflow = "tensorflow"
    $numpy = "numpy"  
    $scipy = "scipy"
    $matplotlib = "matplotlib"
    $pandas = "pandas"
    $transformers = "transformers"
    $langchain = "langchain"
  condition:
    $torch or $tensorflow or $numpy or $scipy or $matplotlib or $pandas or $transformers or $langchain
}
