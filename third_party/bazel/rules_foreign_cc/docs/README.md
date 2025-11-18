<!-- Generated with Stardoc: http://skydoc.bazel.build -->

# Rules ForeignCc Docs

Up to date documentation can be found at: https://bazel-contrib.github.io/rules_foreign_cc/

## Legacy documentation

The sections below exist to maintain links to the previous build rules. Again, the link above
should be used instead.

<a id="#boost_build"></a>

### boost_build

<pre>
boost_build(<a href="#boost_build-name">name</a>, <a href="#boost_build-additional_inputs">additional_inputs</a>, <a href="#boost_build-additional_tools">additional_tools</a>, <a href="#boost_build-alwayslink">alwayslink</a>, <a href="#boost_build-bootstrap_options">bootstrap_options</a>, <a href="#boost_build-data">data</a>, <a href="#boost_build-defines">defines</a>,
            <a href="#boost_build-deps">deps</a>, <a href="#boost_build-env">env</a>, <a href="#boost_build-lib_name">lib_name</a>, <a href="#boost_build-lib_source">lib_source</a>, <a href="#boost_build-linkopts">linkopts</a>, <a href="#boost_build-out_bin_dir">out_bin_dir</a>, <a href="#boost_build-out_binaries">out_binaries</a>, <a href="#boost_build-out_data_dirs">out_data_dirs</a>,
            <a href="#boost_build-out_headers_only">out_headers_only</a>, <a href="#boost_build-out_include_dir">out_include_dir</a>, <a href="#boost_build-out_interface_libs">out_interface_libs</a>, <a href="#boost_build-out_lib_dir">out_lib_dir</a>, <a href="#boost_build-out_shared_libs">out_shared_libs</a>,
            <a href="#boost_build-out_static_libs">out_static_libs</a>, <a href="#boost_build-postfix_script">postfix_script</a>, <a href="#boost_build-tools_deps">tools_deps</a>, <a href="#boost_build-user_options">user_options</a>)
</pre>

Rule for building Boost. Invokes bootstrap.sh and then b2 install.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="boost_build-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="boost_build-additional_inputs"></a>additional_inputs |  Optional additional inputs to be declared as needed for the shell script action.Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="boost_build-additional_tools"></a>additional_tools |  Optional additional tools needed for the building. Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="boost_build-alwayslink"></a>alwayslink |  Optional. if true, link all the object files from the static library, even if they are not used.   | Boolean | optional | False |
| <a id="boost_build-bootstrap_options"></a>bootstrap_options |  any additional flags to pass to bootstrap.sh   | List of strings | optional | [] |
| <a id="boost_build-data"></a>data |  Files needed by this rule at runtime. May list file or rule targets. Generally allows any target.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="boost_build-defines"></a>defines |  Optional compilation definitions to be passed to the dependencies of this library. They are NOT passed to the compiler, you should duplicate them in the configuration options.   | List of strings | optional | [] |
| <a id="boost_build-deps"></a>deps |  Optional dependencies to be copied into the directory structure. Typically those directly required for the external building of the library/binaries. (i.e. those that the external build system will be looking for and paths to which are provided by the calling rule)   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="boost_build-env"></a>env |  Environment variables to set during the build. <code>$(execpath)</code> macros may be used to point at files which are listed as data deps, tools_deps, or additional_tools, but unlike with other rules, these will be replaced with absolute paths to those files, because the build does not run in the exec root. No other macros are supported.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="boost_build-lib_name"></a>lib_name |  Library name. Defines the name of the install directory and the name of the static library, if no output files parameters are defined (any of static_libraries, shared_libraries, interface_libraries, binaries_names) Optional. If not defined, defaults to the target's name.   | String | optional | "" |
| <a id="boost_build-lib_source"></a>lib_source |  Label with source code to build. Typically a filegroup for the source of remote repository. Mandatory.   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |
| <a id="boost_build-linkopts"></a>linkopts |  Optional link options to be passed up to the dependencies of this library   | List of strings | optional | [] |
| <a id="boost_build-out_bin_dir"></a>out_bin_dir |  Optional name of the output subdirectory with the binary files, defaults to 'bin'.   | String | optional | "bin" |
| <a id="boost_build-out_binaries"></a>out_binaries |  Optional names of the resulting binaries.   | List of strings | optional | [] |
| <a id="boost_build-out_data_dirs"></a>out_data_dirs |  Optional names of additional directories created by the build that should be declared as bazel action outputs   | List of strings | optional | [] |
| <a id="boost_build-out_headers_only"></a>out_headers_only |  Flag variable to indicate that the library produces only headers   | Boolean | optional | False |
| <a id="boost_build-out_include_dir"></a>out_include_dir |  Optional name of the output subdirectory with the header files, defaults to 'include'.   | String | optional | "include" |
| <a id="boost_build-out_interface_libs"></a>out_interface_libs |  Optional names of the resulting interface libraries.   | List of strings | optional | [] |
| <a id="boost_build-out_lib_dir"></a>out_lib_dir |  Optional name of the output subdirectory with the library files, defaults to 'lib'.   | String | optional | "lib" |
| <a id="boost_build-out_shared_libs"></a>out_shared_libs |  Optional names of the resulting shared libraries.   | List of strings | optional | [] |
| <a id="boost_build-out_static_libs"></a>out_static_libs |  Optional names of the resulting static libraries. Note that if <code>out_headers_only</code>, <code>out_static_libs</code>, <code>out_shared_libs</code>, and <code>out_binaries</code> are not set, default <code>lib_name.a</code>/<code>lib_name.lib</code> static library is assumed   | List of strings | optional | [] |
| <a id="boost_build-postfix_script"></a>postfix_script |  Optional part of the shell script to be added after the make commands   | String | optional | "" |
| <a id="boost_build-tools_deps"></a>tools_deps |  Optional tools to be copied into the directory structure. Similar to deps, those directly required for the external building of the library/binaries.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="boost_build-user_options"></a>user_options |  any additional flags to pass to b2   | List of strings | optional | [] |


<a id="#cmake"></a>

### cmake

<pre>
cmake(<a href="#cmake-name">name</a>, <a href="#cmake-additional_inputs">additional_inputs</a>, <a href="#cmake-additional_tools">additional_tools</a>, <a href="#cmake-alwayslink">alwayslink</a>, <a href="#cmake-build_args">build_args</a>, <a href="#cmake-cache_entries">cache_entries</a>, <a href="#cmake-data">data</a>,
      <a href="#cmake-defines">defines</a>, <a href="#cmake-deps">deps</a>, <a href="#cmake-env">env</a>, <a href="#cmake-env_vars">env_vars</a>, <a href="#cmake-generate_args">generate_args</a>, <a href="#cmake-generate_crosstool_file">generate_crosstool_file</a>, <a href="#cmake-install">install</a>, <a href="#cmake-install_args">install_args</a>,
      <a href="#cmake-lib_name">lib_name</a>, <a href="#cmake-lib_source">lib_source</a>, <a href="#cmake-linkopts">linkopts</a>, <a href="#cmake-out_bin_dir">out_bin_dir</a>, <a href="#cmake-out_binaries">out_binaries</a>, <a href="#cmake-out_data_dirs">out_data_dirs</a>, <a href="#cmake-out_headers_only">out_headers_only</a>,
      <a href="#cmake-out_include_dir">out_include_dir</a>, <a href="#cmake-out_interface_libs">out_interface_libs</a>, <a href="#cmake-out_lib_dir">out_lib_dir</a>, <a href="#cmake-out_shared_libs">out_shared_libs</a>, <a href="#cmake-out_static_libs">out_static_libs</a>,
      <a href="#cmake-postfix_script">postfix_script</a>, <a href="#cmake-targets">targets</a>, <a href="#cmake-tools_deps">tools_deps</a>, <a href="#cmake-working_directory">working_directory</a>)
</pre>

Rule for building external library with CMake.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="cmake-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="cmake-additional_inputs"></a>additional_inputs |  Optional additional inputs to be declared as needed for the shell script action.Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="cmake-additional_tools"></a>additional_tools |  Optional additional tools needed for the building. Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="cmake-alwayslink"></a>alwayslink |  Optional. if true, link all the object files from the static library, even if they are not used.   | Boolean | optional | False |
| <a id="cmake-build_args"></a>build_args |  Arguments for the CMake build command   | List of strings | optional | [] |
| <a id="cmake-cache_entries"></a>cache_entries |  CMake cache entries to initialize (they will be passed with <code>-Dkey=value</code>) Values, defined by the toolchain, will be joined with the values, passed here. (Toolchain values come first)   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="cmake-data"></a>data |  Files needed by this rule at runtime. May list file or rule targets. Generally allows any target.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="cmake-defines"></a>defines |  Optional compilation definitions to be passed to the dependencies of this library. They are NOT passed to the compiler, you should duplicate them in the configuration options.   | List of strings | optional | [] |
| <a id="cmake-deps"></a>deps |  Optional dependencies to be copied into the directory structure. Typically those directly required for the external building of the library/binaries. (i.e. those that the external build system will be looking for and paths to which are provided by the calling rule)   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="cmake-env"></a>env |  Environment variables to set during the build. <code>$(execpath)</code> macros may be used to point at files which are listed as data deps, tools_deps, or additional_tools, but unlike with other rules, these will be replaced with absolute paths to those files, because the build does not run in the exec root. No other macros are supported.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="cmake-env_vars"></a>env_vars |  CMake environment variable values to join with toolchain-defined. For example, additional <code>CXXFLAGS</code>.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="cmake-generate_args"></a>generate_args |  Arguments for CMake's generate command. Arguments should be passed as key/value pairs. eg: <code>["-G Ninja", "--debug-output", "-DFOO=bar"]</code>. Note that unless a generator (<code>-G</code>) argument is provided, the default generators are [Unix Makefiles](https://cmake.org/cmake/help/latest/generator/Unix%20Makefiles.html) for Linux and MacOS and [Ninja](https://cmake.org/cmake/help/latest/generator/Ninja.html) for Windows.   | List of strings | optional | [] |
| <a id="cmake-generate_crosstool_file"></a>generate_crosstool_file |  When True, CMake crosstool file will be generated from the toolchain values, provided cache-entries and env_vars (some values will still be passed as <code>-Dkey=value</code> and environment variables). If <code>CMAKE_TOOLCHAIN_FILE</code> cache entry is passed, specified crosstool file will be used When using this option to cross-compile, it is required to specify <code>CMAKE_SYSTEM_NAME</code> in the cache_entries   | Boolean | optional | True |
| <a id="cmake-install"></a>install |  If True, the <code>cmake --install</code> comand will be performed after a build   | Boolean | optional | True |
| <a id="cmake-install_args"></a>install_args |  Arguments for the CMake install command   | List of strings | optional | [] |
| <a id="cmake-lib_name"></a>lib_name |  Library name. Defines the name of the install directory and the name of the static library, if no output files parameters are defined (any of static_libraries, shared_libraries, interface_libraries, binaries_names) Optional. If not defined, defaults to the target's name.   | String | optional | "" |
| <a id="cmake-lib_source"></a>lib_source |  Label with source code to build. Typically a filegroup for the source of remote repository. Mandatory.   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |
| <a id="cmake-linkopts"></a>linkopts |  Optional link options to be passed up to the dependencies of this library   | List of strings | optional | [] |
| <a id="cmake-out_bin_dir"></a>out_bin_dir |  Optional name of the output subdirectory with the binary files, defaults to 'bin'.   | String | optional | "bin" |
| <a id="cmake-out_binaries"></a>out_binaries |  Optional names of the resulting binaries.   | List of strings | optional | [] |
| <a id="cmake-out_data_dirs"></a>out_data_dirs |  Optional names of additional directories created by the build that should be declared as bazel action outputs   | List of strings | optional | [] |
| <a id="cmake-out_headers_only"></a>out_headers_only |  Flag variable to indicate that the library produces only headers   | Boolean | optional | False |
| <a id="cmake-out_include_dir"></a>out_include_dir |  Optional name of the output subdirectory with the header files, defaults to 'include'.   | String | optional | "include" |
| <a id="cmake-out_interface_libs"></a>out_interface_libs |  Optional names of the resulting interface libraries.   | List of strings | optional | [] |
| <a id="cmake-out_lib_dir"></a>out_lib_dir |  Optional name of the output subdirectory with the library files, defaults to 'lib'.   | String | optional | "lib" |
| <a id="cmake-out_shared_libs"></a>out_shared_libs |  Optional names of the resulting shared libraries.   | List of strings | optional | [] |
| <a id="cmake-out_static_libs"></a>out_static_libs |  Optional names of the resulting static libraries. Note that if <code>out_headers_only</code>, <code>out_static_libs</code>, <code>out_shared_libs</code>, and <code>out_binaries</code> are not set, default <code>lib_name.a</code>/<code>lib_name.lib</code> static library is assumed   | List of strings | optional | [] |
| <a id="cmake-postfix_script"></a>postfix_script |  Optional part of the shell script to be added after the make commands   | String | optional | "" |
| <a id="cmake-targets"></a>targets |  A list of targets with in the foreign build system to produce. An empty string (<code>""</code>) will result in a call to the underlying build system with no explicit target set   | List of strings | optional | [] |
| <a id="cmake-tools_deps"></a>tools_deps |  Optional tools to be copied into the directory structure. Similar to deps, those directly required for the external building of the library/binaries.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="cmake-working_directory"></a>working_directory |  Working directory, with the main CMakeLists.txt (otherwise, the top directory of the lib_source label files is used.)   | String | optional | "" |


<a id="#cmake_tool"></a>

### cmake_tool

<pre>
cmake_tool(<a href="#cmake_tool-name">name</a>, <a href="#cmake_tool-srcs">srcs</a>)
</pre>

Rule for building CMake. Invokes bootstrap script and make install.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="cmake_tool-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="cmake_tool-srcs"></a>srcs |  The target containing the build tool's sources   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |


<a id="#configure_make"></a>

### configure_make

<pre>
configure_make(<a href="#configure_make-name">name</a>, <a href="#configure_make-additional_inputs">additional_inputs</a>, <a href="#configure_make-additional_tools">additional_tools</a>, <a href="#configure_make-alwayslink">alwayslink</a>, <a href="#configure_make-args">args</a>, <a href="#configure_make-autoconf">autoconf</a>,
               <a href="#configure_make-autoconf_env_vars">autoconf_env_vars</a>, <a href="#configure_make-autoconf_options">autoconf_options</a>, <a href="#configure_make-autogen">autogen</a>, <a href="#configure_make-autogen_command">autogen_command</a>, <a href="#configure_make-autogen_env_vars">autogen_env_vars</a>,
               <a href="#configure_make-autogen_options">autogen_options</a>, <a href="#configure_make-autoreconf">autoreconf</a>, <a href="#configure_make-autoreconf_env_vars">autoreconf_env_vars</a>, <a href="#configure_make-autoreconf_options">autoreconf_options</a>,
               <a href="#configure_make-configure_command">configure_command</a>, <a href="#configure_make-configure_env_vars">configure_env_vars</a>, <a href="#configure_make-configure_in_place">configure_in_place</a>, <a href="#configure_make-configure_options">configure_options</a>, <a href="#configure_make-data">data</a>,
               <a href="#configure_make-defines">defines</a>, <a href="#configure_make-deps">deps</a>, <a href="#configure_make-env">env</a>, <a href="#configure_make-install_prefix">install_prefix</a>, <a href="#configure_make-lib_name">lib_name</a>, <a href="#configure_make-lib_source">lib_source</a>, <a href="#configure_make-linkopts">linkopts</a>, <a href="#configure_make-make_commands">make_commands</a>,
               <a href="#configure_make-out_bin_dir">out_bin_dir</a>, <a href="#configure_make-out_binaries">out_binaries</a>, <a href="#configure_make-out_data_dirs">out_data_dirs</a>, <a href="#configure_make-out_headers_only">out_headers_only</a>, <a href="#configure_make-out_include_dir">out_include_dir</a>,
               <a href="#configure_make-out_interface_libs">out_interface_libs</a>, <a href="#configure_make-out_lib_dir">out_lib_dir</a>, <a href="#configure_make-out_shared_libs">out_shared_libs</a>, <a href="#configure_make-out_static_libs">out_static_libs</a>, <a href="#configure_make-postfix_script">postfix_script</a>,
               <a href="#configure_make-targets">targets</a>, <a href="#configure_make-tools_deps">tools_deps</a>)
</pre>

Rule for building external libraries with configure-make pattern. Some 'configure' script is invoked with --prefix=install (by default), and other parameters for compilation and linking, taken from Bazel C/C++ toolchain and passed dependencies. After configuration, GNU Make is called.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="configure_make-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="configure_make-additional_inputs"></a>additional_inputs |  Optional additional inputs to be declared as needed for the shell script action.Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="configure_make-additional_tools"></a>additional_tools |  Optional additional tools needed for the building. Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="configure_make-alwayslink"></a>alwayslink |  Optional. if true, link all the object files from the static library, even if they are not used.   | Boolean | optional | False |
| <a id="configure_make-args"></a>args |  A list of arguments to pass to the call to <code>make</code>   | List of strings | optional | [] |
| <a id="configure_make-autoconf"></a>autoconf |  Set to True if 'autoconf' should be invoked before 'configure', currently requires <code>configure_in_place</code> to be True.   | Boolean | optional | False |
| <a id="configure_make-autoconf_env_vars"></a>autoconf_env_vars |  Environment variables to be set for 'autoconf' invocation.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="configure_make-autoconf_options"></a>autoconf_options |  Any options to be put in the 'autoconf.sh' command line.   | List of strings | optional | [] |
| <a id="configure_make-autogen"></a>autogen |  Set to True if 'autogen.sh' should be invoked before 'configure', currently requires <code>configure_in_place</code> to be True.   | Boolean | optional | False |
| <a id="configure_make-autogen_command"></a>autogen_command |  The name of the autogen script file, default: autogen.sh. Many projects use autogen.sh however the Autotools FAQ recommends bootstrap so we provide this option to support that.   | String | optional | "autogen.sh" |
| <a id="configure_make-autogen_env_vars"></a>autogen_env_vars |  Environment variables to be set for 'autogen' invocation.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="configure_make-autogen_options"></a>autogen_options |  Any options to be put in the 'autogen.sh' command line.   | List of strings | optional | [] |
| <a id="configure_make-autoreconf"></a>autoreconf |  Set to True if 'autoreconf' should be invoked before 'configure.', currently requires <code>configure_in_place</code> to be True.   | Boolean | optional | False |
| <a id="configure_make-autoreconf_env_vars"></a>autoreconf_env_vars |  Environment variables to be set for 'autoreconf' invocation.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="configure_make-autoreconf_options"></a>autoreconf_options |  Any options to be put in the 'autoreconf.sh' command line.   | List of strings | optional | [] |
| <a id="configure_make-configure_command"></a>configure_command |  The name of the configuration script file, default: configure. The file must be in the root of the source directory.   | String | optional | "configure" |
| <a id="configure_make-configure_env_vars"></a>configure_env_vars |  Environment variables to be set for the 'configure' invocation.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="configure_make-configure_in_place"></a>configure_in_place |  Set to True if 'configure' should be invoked in place, i.e. from its enclosing directory.   | Boolean | optional | False |
| <a id="configure_make-configure_options"></a>configure_options |  Any options to be put on the 'configure' command line.   | List of strings | optional | [] |
| <a id="configure_make-data"></a>data |  Files needed by this rule at runtime. May list file or rule targets. Generally allows any target.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="configure_make-defines"></a>defines |  Optional compilation definitions to be passed to the dependencies of this library. They are NOT passed to the compiler, you should duplicate them in the configuration options.   | List of strings | optional | [] |
| <a id="configure_make-deps"></a>deps |  Optional dependencies to be copied into the directory structure. Typically those directly required for the external building of the library/binaries. (i.e. those that the external build system will be looking for and paths to which are provided by the calling rule)   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="configure_make-env"></a>env |  Environment variables to set during the build. <code>$(execpath)</code> macros may be used to point at files which are listed as data deps, tools_deps, or additional_tools, but unlike with other rules, these will be replaced with absolute paths to those files, because the build does not run in the exec root. No other macros are supported.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="configure_make-install_prefix"></a>install_prefix |  Install prefix, i.e. relative path to where to install the result of the build. Passed to the 'configure' script with --prefix flag.   | String | optional | "" |
| <a id="configure_make-lib_name"></a>lib_name |  Library name. Defines the name of the install directory and the name of the static library, if no output files parameters are defined (any of static_libraries, shared_libraries, interface_libraries, binaries_names) Optional. If not defined, defaults to the target's name.   | String | optional | "" |
| <a id="configure_make-lib_source"></a>lib_source |  Label with source code to build. Typically a filegroup for the source of remote repository. Mandatory.   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |
| <a id="configure_make-linkopts"></a>linkopts |  Optional link options to be passed up to the dependencies of this library   | List of strings | optional | [] |
| <a id="configure_make-make_commands"></a>make_commands |  Optional make commands.   | List of strings | optional | ["make", "make install"] |
| <a id="configure_make-out_bin_dir"></a>out_bin_dir |  Optional name of the output subdirectory with the binary files, defaults to 'bin'.   | String | optional | "bin" |
| <a id="configure_make-out_binaries"></a>out_binaries |  Optional names of the resulting binaries.   | List of strings | optional | [] |
| <a id="configure_make-out_data_dirs"></a>out_data_dirs |  Optional names of additional directories created by the build that should be declared as bazel action outputs   | List of strings | optional | [] |
| <a id="configure_make-out_headers_only"></a>out_headers_only |  Flag variable to indicate that the library produces only headers   | Boolean | optional | False |
| <a id="configure_make-out_include_dir"></a>out_include_dir |  Optional name of the output subdirectory with the header files, defaults to 'include'.   | String | optional | "include" |
| <a id="configure_make-out_interface_libs"></a>out_interface_libs |  Optional names of the resulting interface libraries.   | List of strings | optional | [] |
| <a id="configure_make-out_lib_dir"></a>out_lib_dir |  Optional name of the output subdirectory with the library files, defaults to 'lib'.   | String | optional | "lib" |
| <a id="configure_make-out_shared_libs"></a>out_shared_libs |  Optional names of the resulting shared libraries.   | List of strings | optional | [] |
| <a id="configure_make-out_static_libs"></a>out_static_libs |  Optional names of the resulting static libraries. Note that if <code>out_headers_only</code>, <code>out_static_libs</code>, <code>out_shared_libs</code>, and <code>out_binaries</code> are not set, default <code>lib_name.a</code>/<code>lib_name.lib</code> static library is assumed   | List of strings | optional | [] |
| <a id="configure_make-postfix_script"></a>postfix_script |  Optional part of the shell script to be added after the make commands   | String | optional | "" |
| <a id="configure_make-targets"></a>targets |  A list of targets within the foreign build system to produce. An empty string (<code>""</code>) will result in a call to the underlying build system with no explicit target set   | List of strings | optional | ["", "install"] |
| <a id="configure_make-tools_deps"></a>tools_deps |  Optional tools to be copied into the directory structure. Similar to deps, those directly required for the external building of the library/binaries.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |


<a id="#make"></a>

### make

<pre>
make(<a href="#make-name">name</a>, <a href="#make-additional_inputs">additional_inputs</a>, <a href="#make-additional_tools">additional_tools</a>, <a href="#make-alwayslink">alwayslink</a>, <a href="#make-args">args</a>, <a href="#make-data">data</a>, <a href="#make-defines">defines</a>, <a href="#make-deps">deps</a>, <a href="#make-env">env</a>,
     <a href="#make-lib_name">lib_name</a>, <a href="#make-lib_source">lib_source</a>, <a href="#make-linkopts">linkopts</a>, <a href="#make-out_bin_dir">out_bin_dir</a>, <a href="#make-out_binaries">out_binaries</a>, <a href="#make-out_data_dirs">out_data_dirs</a>, <a href="#make-out_headers_only">out_headers_only</a>,
     <a href="#make-out_include_dir">out_include_dir</a>, <a href="#make-out_interface_libs">out_interface_libs</a>, <a href="#make-out_lib_dir">out_lib_dir</a>, <a href="#make-out_shared_libs">out_shared_libs</a>, <a href="#make-out_static_libs">out_static_libs</a>,
     <a href="#make-postfix_script">postfix_script</a>, <a href="#make-targets">targets</a>, <a href="#make-tools_deps">tools_deps</a>)
</pre>

Rule for building external libraries with GNU Make. GNU Make commands (make and make install by default) are invoked with prefix="install" (by default), and other environment variables for compilation and linking, taken from Bazel C/C++ toolchain and passed dependencies.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="make-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="make-additional_inputs"></a>additional_inputs |  Optional additional inputs to be declared as needed for the shell script action.Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="make-additional_tools"></a>additional_tools |  Optional additional tools needed for the building. Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="make-alwayslink"></a>alwayslink |  Optional. if true, link all the object files from the static library, even if they are not used.   | Boolean | optional | False |
| <a id="make-args"></a>args |  A list of arguments to pass to the call to <code>make</code>   | List of strings | optional | [] |
| <a id="make-data"></a>data |  Files needed by this rule at runtime. May list file or rule targets. Generally allows any target.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="make-defines"></a>defines |  Optional compilation definitions to be passed to the dependencies of this library. They are NOT passed to the compiler, you should duplicate them in the configuration options.   | List of strings | optional | [] |
| <a id="make-deps"></a>deps |  Optional dependencies to be copied into the directory structure. Typically those directly required for the external building of the library/binaries. (i.e. those that the external build system will be looking for and paths to which are provided by the calling rule)   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="make-env"></a>env |  Environment variables to set during the build. <code>$(execpath)</code> macros may be used to point at files which are listed as data deps, tools_deps, or additional_tools, but unlike with other rules, these will be replaced with absolute paths to those files, because the build does not run in the exec root. No other macros are supported.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="make-lib_name"></a>lib_name |  Library name. Defines the name of the install directory and the name of the static library, if no output files parameters are defined (any of static_libraries, shared_libraries, interface_libraries, binaries_names) Optional. If not defined, defaults to the target's name.   | String | optional | "" |
| <a id="make-lib_source"></a>lib_source |  Label with source code to build. Typically a filegroup for the source of remote repository. Mandatory.   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |
| <a id="make-linkopts"></a>linkopts |  Optional link options to be passed up to the dependencies of this library   | List of strings | optional | [] |
| <a id="make-out_bin_dir"></a>out_bin_dir |  Optional name of the output subdirectory with the binary files, defaults to 'bin'.   | String | optional | "bin" |
| <a id="make-out_binaries"></a>out_binaries |  Optional names of the resulting binaries.   | List of strings | optional | [] |
| <a id="make-out_data_dirs"></a>out_data_dirs |  Optional names of additional directories created by the build that should be declared as bazel action outputs   | List of strings | optional | [] |
| <a id="make-out_headers_only"></a>out_headers_only |  Flag variable to indicate that the library produces only headers   | Boolean | optional | False |
| <a id="make-out_include_dir"></a>out_include_dir |  Optional name of the output subdirectory with the header files, defaults to 'include'.   | String | optional | "include" |
| <a id="make-out_interface_libs"></a>out_interface_libs |  Optional names of the resulting interface libraries.   | List of strings | optional | [] |
| <a id="make-out_lib_dir"></a>out_lib_dir |  Optional name of the output subdirectory with the library files, defaults to 'lib'.   | String | optional | "lib" |
| <a id="make-out_shared_libs"></a>out_shared_libs |  Optional names of the resulting shared libraries.   | List of strings | optional | [] |
| <a id="make-out_static_libs"></a>out_static_libs |  Optional names of the resulting static libraries. Note that if <code>out_headers_only</code>, <code>out_static_libs</code>, <code>out_shared_libs</code>, and <code>out_binaries</code> are not set, default <code>lib_name.a</code>/<code>lib_name.lib</code> static library is assumed   | List of strings | optional | [] |
| <a id="make-postfix_script"></a>postfix_script |  Optional part of the shell script to be added after the make commands   | String | optional | "" |
| <a id="make-targets"></a>targets |  A list of targets within the foreign build system to produce. An empty string (<code>""</code>) will result in a call to the underlying build system with no explicit target set   | List of strings | optional | ["", "install"] |
| <a id="make-tools_deps"></a>tools_deps |  Optional tools to be copied into the directory structure. Similar to deps, those directly required for the external building of the library/binaries.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |


<a id="#make_tool"></a>

## make_tool

<pre>
make_tool(<a href="#make_tool-name">name</a>, <a href="#make_tool-srcs">srcs</a>)
</pre>

Rule for building Make. Invokes configure script and make install.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="make_tool-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="make_tool-srcs"></a>srcs |  The target containing the build tool's sources   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |


<a id="#native_tool_toolchain"></a>

### native_tool_toolchain

<pre>
native_tool_toolchain(<a href="#native_tool_toolchain-name">name</a>, <a href="#native_tool_toolchain-path">path</a>, <a href="#native_tool_toolchain-target">target</a>)
</pre>

Rule for defining the toolchain data of the native tools (cmake, ninja), to be used by rules_foreign_cc with toolchain types `@rules_foreign_cc//toolchains:cmake_toolchain` and `@rules_foreign_cc//toolchains:ninja_toolchain`.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="native_tool_toolchain-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="native_tool_toolchain-path"></a>path |  Absolute path to the tool in case the tool is preinstalled on the machine. Relative path to the tool in case the tool is built as part of a build; the path should be relative to the bazel-genfiles, i.e. it should start with the name of the top directory of the built tree artifact. (Please see the example <code>//examples:built_cmake_toolchain</code>)   | String | optional | "" |
| <a id="native_tool_toolchain-target"></a>target |  If the tool is preinstalled, must be None. If the tool is built as part of the build, the corresponding build target, which should produce the tree artifact with the binary to call.   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | optional | None |


<a id="#ninja"></a>

### ninja

<pre>
ninja(<a href="#ninja-name">name</a>, <a href="#ninja-additional_inputs">additional_inputs</a>, <a href="#ninja-additional_tools">additional_tools</a>, <a href="#ninja-alwayslink">alwayslink</a>, <a href="#ninja-args">args</a>, <a href="#ninja-data">data</a>, <a href="#ninja-defines">defines</a>, <a href="#ninja-deps">deps</a>, <a href="#ninja-directory">directory</a>,
      <a href="#ninja-env">env</a>, <a href="#ninja-lib_name">lib_name</a>, <a href="#ninja-lib_source">lib_source</a>, <a href="#ninja-linkopts">linkopts</a>, <a href="#ninja-out_bin_dir">out_bin_dir</a>, <a href="#ninja-out_binaries">out_binaries</a>, <a href="#ninja-out_data_dirs">out_data_dirs</a>, <a href="#ninja-out_headers_only">out_headers_only</a>,
      <a href="#ninja-out_include_dir">out_include_dir</a>, <a href="#ninja-out_interface_libs">out_interface_libs</a>, <a href="#ninja-out_lib_dir">out_lib_dir</a>, <a href="#ninja-out_shared_libs">out_shared_libs</a>, <a href="#ninja-out_static_libs">out_static_libs</a>,
      <a href="#ninja-postfix_script">postfix_script</a>, <a href="#ninja-targets">targets</a>, <a href="#ninja-tools_deps">tools_deps</a>)
</pre>

Rule for building external libraries with [Ninja](https://ninja-build.org/).

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="ninja-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="ninja-additional_inputs"></a>additional_inputs |  Optional additional inputs to be declared as needed for the shell script action.Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="ninja-additional_tools"></a>additional_tools |  Optional additional tools needed for the building. Not used by the shell script part in cc_external_rule_impl.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="ninja-alwayslink"></a>alwayslink |  Optional. if true, link all the object files from the static library, even if they are not used.   | Boolean | optional | False |
| <a id="ninja-args"></a>args |  A list of arguments to pass to the call to <code>ninja</code>   | List of strings | optional | [] |
| <a id="ninja-data"></a>data |  Files needed by this rule at runtime. May list file or rule targets. Generally allows any target.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="ninja-defines"></a>defines |  Optional compilation definitions to be passed to the dependencies of this library. They are NOT passed to the compiler, you should duplicate them in the configuration options.   | List of strings | optional | [] |
| <a id="ninja-deps"></a>deps |  Optional dependencies to be copied into the directory structure. Typically those directly required for the external building of the library/binaries. (i.e. those that the external build system will be looking for and paths to which are provided by the calling rule)   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |
| <a id="ninja-directory"></a>directory |  A directory to pass as the <code>-C</code> argument. The rule will always use the root directory of the <code>lib_sources</code> attribute if this attribute is not set   | String | optional | "" |
| <a id="ninja-env"></a>env |  Environment variables to set during the build. <code>$(execpath)</code> macros may be used to point at files which are listed as data deps, tools_deps, or additional_tools, but unlike with other rules, these will be replaced with absolute paths to those files, because the build does not run in the exec root. No other macros are supported.   | <a href="https://bazel.build/docs/skylark/lib/dict.html">Dictionary: String -> String</a> | optional | {} |
| <a id="ninja-lib_name"></a>lib_name |  Library name. Defines the name of the install directory and the name of the static library, if no output files parameters are defined (any of static_libraries, shared_libraries, interface_libraries, binaries_names) Optional. If not defined, defaults to the target's name.   | String | optional | "" |
| <a id="ninja-lib_source"></a>lib_source |  Label with source code to build. Typically a filegroup for the source of remote repository. Mandatory.   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |
| <a id="ninja-linkopts"></a>linkopts |  Optional link options to be passed up to the dependencies of this library   | List of strings | optional | [] |
| <a id="ninja-out_bin_dir"></a>out_bin_dir |  Optional name of the output subdirectory with the binary files, defaults to 'bin'.   | String | optional | "bin" |
| <a id="ninja-out_binaries"></a>out_binaries |  Optional names of the resulting binaries.   | List of strings | optional | [] |
| <a id="ninja-out_data_dirs"></a>out_data_dirs |  Optional names of additional directories created by the build that should be declared as bazel action outputs   | List of strings | optional | [] |
| <a id="ninja-out_headers_only"></a>out_headers_only |  Flag variable to indicate that the library produces only headers   | Boolean | optional | False |
| <a id="ninja-out_include_dir"></a>out_include_dir |  Optional name of the output subdirectory with the header files, defaults to 'include'.   | String | optional | "include" |
| <a id="ninja-out_interface_libs"></a>out_interface_libs |  Optional names of the resulting interface libraries.   | List of strings | optional | [] |
| <a id="ninja-out_lib_dir"></a>out_lib_dir |  Optional name of the output subdirectory with the library files, defaults to 'lib'.   | String | optional | "lib" |
| <a id="ninja-out_shared_libs"></a>out_shared_libs |  Optional names of the resulting shared libraries.   | List of strings | optional | [] |
| <a id="ninja-out_static_libs"></a>out_static_libs |  Optional names of the resulting static libraries. Note that if <code>out_headers_only</code>, <code>out_static_libs</code>, <code>out_shared_libs</code>, and <code>out_binaries</code> are not set, default <code>lib_name.a</code>/<code>lib_name.lib</code> static library is assumed   | List of strings | optional | [] |
| <a id="ninja-postfix_script"></a>postfix_script |  Optional part of the shell script to be added after the make commands   | String | optional | "" |
| <a id="ninja-targets"></a>targets |  A list of targets with in the foreign build system to produce. An empty string (<code>""</code>) will result in a call to the underlying build system with no explicit target set   | List of strings | optional | [] |
| <a id="ninja-tools_deps"></a>tools_deps |  Optional tools to be copied into the directory structure. Similar to deps, those directly required for the external building of the library/binaries.   | <a href="https://bazel.build/docs/build-ref.html#labels">List of labels</a> | optional | [] |


<a id="#ninja_tool"></a>

### ninja_tool

<pre>
ninja_tool(<a href="#ninja_tool-name">name</a>, <a href="#ninja_tool-srcs">srcs</a>)
</pre>

Rule for building Ninja. Invokes configure script.

**ATTRIBUTES**


| Name  | Description | Type | Mandatory | Default |
| :------------- | :------------- | :------------- | :------------- | :------------- |
| <a id="ninja_tool-name"></a>name |  A unique name for this target.   | <a href="https://bazel.build/docs/build-ref.html#name">Name</a> | required |  |
| <a id="ninja_tool-srcs"></a>srcs |  The target containing the build tool's sources   | <a href="https://bazel.build/docs/build-ref.html#labels">Label</a> | required |  |


<a id="#ForeignCcArtifact"></a>

### ForeignCcArtifact

<pre>
ForeignCcArtifact(<a href="#ForeignCcArtifact-bin_dir_name">bin_dir_name</a>, <a href="#ForeignCcArtifact-gen_dir">gen_dir</a>, <a href="#ForeignCcArtifact-include_dir_name">include_dir_name</a>, <a href="#ForeignCcArtifact-lib_dir_name">lib_dir_name</a>)
</pre>

Groups information about the external library install directory,
and relative bin, include and lib directories.

Serves to pass transitive information about externally built artifacts up the dependency chain.

Can not be used as a top-level provider.
Instances of ForeignCcArtifact are incapsulated in a depset ForeignCcDeps#artifacts.

**FIELDS**


| Name  | Description |
| :------------- | :------------- |
| <a id="ForeignCcArtifact-bin_dir_name"></a>bin_dir_name |  Bin directory, relative to install directory    |
| <a id="ForeignCcArtifact-gen_dir"></a>gen_dir |  Install directory    |
| <a id="ForeignCcArtifact-include_dir_name"></a>include_dir_name |  Include directory, relative to install directory    |
| <a id="ForeignCcArtifact-lib_dir_name"></a>lib_dir_name |  Lib directory, relative to install directory    |


<a id="#ForeignCcDeps"></a>

### ForeignCcDeps

<pre>
ForeignCcDeps(<a href="#ForeignCcDeps-artifacts">artifacts</a>)
</pre>

Provider to pass transitive information about external libraries.

**FIELDS**


| Name  | Description |
| :------------- | :------------- |
| <a id="ForeignCcDeps-artifacts"></a>artifacts |  Depset of ForeignCcArtifact    |


<a id="#ToolInfo"></a>

### ToolInfo

<pre>
ToolInfo(<a href="#ToolInfo-path">path</a>, <a href="#ToolInfo-target">target</a>)
</pre>

Information about the native tool

**FIELDS**


| Name  | Description |
| :------------- | :------------- |
| <a id="ToolInfo-path"></a>path |  Absolute path to the tool in case the tool is preinstalled on the machine. Relative path to the tool in case the tool is built as part of a build; the path should be relative to the bazel-genfiles, i.e. it should start with the name of the top directory of the built tree artifact. (Please see the example <code>//examples:built_cmake_toolchain</code>)    |
| <a id="ToolInfo-target"></a>target |  If the tool is preinstalled, must be None. If the tool is built as part of the build, the corresponding build target, which should produce the tree artifact with the binary to call.    |


<a id="#rules_foreign_cc_dependencies"></a>

### rules_foreign_cc_dependencies

<pre>
rules_foreign_cc_dependencies(<a href="#rules_foreign_cc_dependencies-native_tools_toolchains">native_tools_toolchains</a>, <a href="#rules_foreign_cc_dependencies-register_default_tools">register_default_tools</a>, <a href="#rules_foreign_cc_dependencies-cmake_version">cmake_version</a>,
                              <a href="#rules_foreign_cc_dependencies-make_version">make_version</a>, <a href="#rules_foreign_cc_dependencies-ninja_version">ninja_version</a>, <a href="#rules_foreign_cc_dependencies-register_preinstalled_tools">register_preinstalled_tools</a>,
                              <a href="#rules_foreign_cc_dependencies-register_built_tools">register_built_tools</a>)
</pre>

Call this function from the WORKSPACE file to initialize rules_foreign_cc     dependencies and let neccesary code generation happen     (Code generation is needed to support different variants of the C++ Starlark API.).

**PARAMETERS**


| Name  | Description | Default Value |
| :------------- | :------------- | :------------- |
| <a id="rules_foreign_cc_dependencies-native_tools_toolchains"></a>native_tools_toolchains |  pass the toolchains for toolchain types     '@rules_foreign_cc//toolchains:cmake_toolchain' and     '@rules_foreign_cc//toolchains:ninja_toolchain' with the needed platform constraints.     If you do not pass anything, registered default toolchains will be selected (see below).   |  <code>[]</code> |
| <a id="rules_foreign_cc_dependencies-register_default_tools"></a>register_default_tools |  If True, the cmake and ninja toolchains, calling corresponding     preinstalled binaries by name (cmake, ninja) will be registered after     'native_tools_toolchains' without any platform constraints. The default is True.   |  <code>True</code> |
| <a id="rules_foreign_cc_dependencies-cmake_version"></a>cmake_version |  The target version of the cmake toolchain if <code>register_default_tools</code>     or <code>register_built_tools</code> is set to <code>True</code>.   |  <code>"3.22.0"</code> |
| <a id="rules_foreign_cc_dependencies-make_version"></a>make_version |  The target version of the default make toolchain if <code>register_built_tools</code>     is set to <code>True</code>.   |  <code>"4.3"</code> |
| <a id="rules_foreign_cc_dependencies-ninja_version"></a>ninja_version |  The target version of the ninja toolchain if <code>register_default_tools</code>     or <code>register_built_tools</code> is set to <code>True</code>.   |  <code>"1.10.2"</code> |
| <a id="rules_foreign_cc_dependencies-register_preinstalled_tools"></a>register_preinstalled_tools |  If true, toolchains will be registered for the native built tools     installed on the exec host   |  <code>True</code> |
| <a id="rules_foreign_cc_dependencies-register_built_tools"></a>register_built_tools |  If true, toolchains that build the tools from source are registered   |  <code>True</code> |
