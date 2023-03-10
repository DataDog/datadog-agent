#!/bin/bash
# (C) Datadog, Inc. 2010-present
# All rights reserved
# Licensed under Apache-2.0 License (see LICENSE)
# Datadog Agent installation script: install and set up the Agent on supported Linux distributions
# using the package manager and Datadog repositories.

set -e

echo -e "\033[33m
 install_script.sh is deprecated. Please use one of
 
 * https://s3.amazonaws.com/dd-agent/scripts/install_script_agent6.sh to install Agent 6
 * https://s3.amazonaws.com/dd-agent/scripts/install_script_agent7.sh to install Agent 7
\033[0m"

install_script_version=1.13.0.deprecated
logfile="ddagent-install.log"
support_email=support@datadoghq.com

LEGACY_ETCDIR="/etc/dd-agent"
LEGACY_CONF="$LEGACY_ETCDIR/datadog.conf"

# DATADOG_APT_KEY_CURRENT.public always contains key used to sign current
# repodata and newly released packages
# DATADOG_APT_KEY_382E94DE.public expires in 2022
# DATADOG_APT_KEY_F14F620E.public expires in 2032
APT_GPG_KEYS=("DATADOG_APT_KEY_CURRENT.public" "DATADOG_APT_KEY_F14F620E.public" "DATADOG_APT_KEY_382E94DE.public")

# DATADOG_RPM_KEY_CURRENT.public always contains key used to sign current
# repodata and newly released packages
# DATADOG_RPM_KEY_E09422B3.public expires in 2022
# DATADOG_RPM_KEY_FD4BF915.public expires in 2024
RPM_GPG_KEYS=("DATADOG_RPM_KEY_CURRENT.public" "DATADOG_RPM_KEY_E09422B3.public" "DATADOG_RPM_KEY_FD4BF915.public")

# DATADOG_RPM_KEY.public (4172A230) was only useful to install old (< 6.14) Agent packages.
# We no longer add it and we explicitly remove it.
RPM_GPG_KEYS_TO_REMOVE=("gpg-pubkey-4172a230-55dd14f6")

# Set up a named pipe for logging
npipe=/tmp/$$.tmp
mknod $npipe p

# Log all output to a log for error checking
tee <$npipe $logfile &
exec 1>&-
exec 1>$npipe 2>&1
trap 'rm -f $npipe' EXIT

function fallback_msg(){
  printf "
If you are still having problems, please send an email to $support_email
with the contents of $logfile and any information you think would be
useful and we will do our very best to help you solve your problem.\n"
}

function report(){
  if curl -f -s \
    --data-urlencode "os=${OS}" \
    --data-urlencode "version=${agent_major_version}" \
    --data-urlencode "log=$(cat $logfile)" \
    --data-urlencode "email=${email}" \
    --data-urlencode "apikey=${apikey}" \
    "$report_failure_url"; then
   printf "A notification has been sent to Datadog with the contents of $logfile\n"
  else
    printf "Unable to send the notification (curl v7.18 or newer is required)"
  fi
}

function on_read_error() {
  printf "Timed out or input EOF reached, assuming 'No'\n"
  yn="n"
}

function get_email() {
  emaillocalpart='^[a-zA-Z0-9][a-zA-Z0-9._%+-]{0,63}'
  hostnamepart='[a-zA-Z0-9.-]+\.[a-zA-Z]+'
  email_regex="$emaillocalpart@$hostnamepart"
  cntr=0
  until [[ "$cntr" -eq 3 ]]
  do
      read -p "Enter an email address so we can follow up: " -r email
      if [[ "$email" =~ $email_regex ]]; then
        isEmailValid=true
        break
      else
        ((cntr=cntr+1))
        echo -e "\033[33m($cntr/3) Email address invalid: $email\033[0m\n"
      fi
  done
}

function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the $nice_flavor.

Troubleshooting and basic usage information for the $nice_flavor are available at:

    https://docs.datadoghq.com/agent/basic_agent_usage/\n\033[0m\n"

    if ! tty -s; then
      fallback_msg
      exit 1;
    fi

    if [ "$site" == "ddog-gov.com" ]; then
      fallback_msg
      exit 1;
    fi

    while true; do
        read -t 60 -p  "Do you want to send a failure report to Datadog (including $logfile)? (y/[n]) " -r yn || on_read_error
        case $yn in
          [Yy]* )
            get_email
            if [[ -n "$isEmailValid" ]]; then
              report
            fi
            fallback_msg
            break;;
          [Nn]*|"" )
            fallback_msg
            break;;
          * )
            printf "Please answer yes or no.\n"
            ;;
        esac
    done
}
trap on_error ERR

function verify_agent_version(){
    local ver_separator="$1"
    if [ -z "$agent_version_custom" ]; then
        echo -e "
  \033[33mWarning: Specified version not found: $agent_major_version.$agent_minor_version
  Check available versions at: https://github.com/DataDog/datadog-agent/blob/main/CHANGELOG.rst\033[0m"
        fallback_msg
        exit 1;
    else
        agent_flavor+="$ver_separator$agent_version_custom"
    fi
}

function remove_rpm_gpg_keys() {
    local sudo_cmd="$1"
    shift
    local old_keys=("$@")
    for key in "${old_keys[@]}"; do
        if $sudo_cmd rpm -q "$key" 1>/dev/null 2>/dev/null; then
            echo -e "\033[34m\nRemoving old RPM key $key from the RPM database\n\033[0m"
            $sudo_cmd rpm --erase "$key"
        fi
    done
}

echo -e "\033[34m\n* Datadog Agent install script v${install_script_version}\n\033[0m"

hostname=
if [ -n "$DD_HOSTNAME" ]; then
    hostname=$DD_HOSTNAME
fi

site=
if [ -n "$DD_SITE" ]; then
    site="$DD_SITE"
fi

apikey=
if [ -n "$DD_API_KEY" ]; then
    apikey=$DD_API_KEY
fi

no_start=
if [ -n "$DD_INSTALL_ONLY" ]; then
    no_start=true
fi

host_tags=
# comma-separated list of tags
if [ -n "$DD_HOST_TAGS" ]; then
    host_tags=$DD_HOST_TAGS
fi

if [ -n "$REPO_URL" ]; then
    repository_url=$REPO_URL
else
    repository_url="datadoghq.com"
fi

if [ -n "$TESTING_KEYS_URL" ]; then
  keys_url=$TESTING_KEYS_URL
else
  keys_url="keys.datadoghq.com"
fi

if [ -n "$TESTING_YUM_URL" ]; then
  yum_url=$TESTING_YUM_URL
else
  yum_url="yum.${repository_url}"
fi

# We turn off `repo_gpgcheck` for custom REPO_URL, unless explicitly turned
# on via DD_RPM_REPO_GPGCHECK.
# There is more logic for redhat/suse in their specific code branches below
rpm_repo_gpgcheck=
if [ -n "$DD_RPM_REPO_GPGCHECK" ]; then
    rpm_repo_gpgcheck=$DD_RPM_REPO_GPGCHECK
else
    if [ -n "$REPO_URL" ]; then
        rpm_repo_gpgcheck=0
    fi
fi

if [ -n "$TESTING_APT_URL" ]; then
  apt_url=$TESTING_APT_URL
else
  apt_url="apt.${repository_url}"
fi

upgrade=
if [ -n "$DD_UPGRADE" ]; then
  upgrade=$DD_UPGRADE
fi

fips_mode=
if [ -n "$DD_FIPS_MODE" ]; then
  fips_mode=$DD_FIPS_MODE
fi

agent_flavor="datadog-agent"
if [ -n "$DD_AGENT_FLAVOR" ]; then
    agent_flavor=$DD_AGENT_FLAVOR #Eg: datadog-iot-agent
fi

declare -A flavor_to_readable
flavor_to_readable=(
    ["datadog-agent"]="Datadog Agent"
    ["datadog-iot-agent"]="Datadog IoT Agent"
    ["datadog-dogstatsd"]="Datadog Dogstatsd"
    ["datadog-fips-proxy"]="Datadog FIPS Proxy"
    ["datadog-heroku-agent"]="Datadog Heroku Agent"
)
nice_flavor=${flavor_to_readable[$agent_flavor]}

if [ -z "$nice_flavor" ]; then
    echo -e "\033[33mUnknown DD_AGENT_FLAVOR \"$agent_flavor\"\033[0m"
    fallback_msg
    exit 1;
fi

declare -A flavor_to_system_service
flavor_to_system_service=(
    ["datadog-dogstatsd"]="datadog-dogstatsd"
)
system_service=${flavor_to_system_service[$agent_flavor]:-datadog-agent}

declare -a services
services=("$system_service")
if [ -n "$fips_mode" ]; then
  services+=("datadog-fips-proxy")
fi

declare -A flavor_to_etcdir
flavor_to_etcdir=(
    ["datadog-dogstatsd"]="/etc/datadog-dogstatsd"
)
etcdir=${flavor_to_etcdir[$agent_flavor]:-/etc/datadog-agent}
etcdirfips=/etc/datadog-fips-proxy

declare -A flavor_to_config
flavor_to_config=(
    ["datadog-dogstatsd"]="$etcdir/dogstatsd.yaml"
)
config_file=${flavor_to_config[$agent_flavor]:-$etcdir/datadog.yaml}
config_file_fips=$etcdirfips/datadog-fips-proxy.cfg

agent_major_version=6
if [ -n "$DD_AGENT_MAJOR_VERSION" ]; then
  if [ "$DD_AGENT_MAJOR_VERSION" != "6" ] && [ "$DD_AGENT_MAJOR_VERSION" != "7" ]; then
    echo "DD_AGENT_MAJOR_VERSION must be either 6 or 7. Current value: $DD_AGENT_MAJOR_VERSION"
    exit 1;
  fi
  agent_major_version=$DD_AGENT_MAJOR_VERSION
else
  if [ "$agent_flavor" == "datadog-agent" ] ; then
    echo -e "\033[33mWarning: DD_AGENT_MAJOR_VERSION not set. Installing $nice_flavor version 6 by default.\033[0m"
  else
    echo -e "\033[33mWarning: DD_AGENT_MAJOR_VERSION not set. Installing $nice_flavor version 7 by default.\033[0m"
    agent_major_version=7
  fi
fi

if [ -n "$DD_AGENT_MINOR_VERSION" ]; then
  # Examples:
  #  - 20   = defaults to highest patch version x.20.2
  #  - 20.0 = sets explicit patch version x.20.0
  # Note: Specifying an invalid minor version will terminate the script.
  agent_minor_version=$DD_AGENT_MINOR_VERSION
  # remove the patch version if the minor version includes it (eg: 33.1 -> 33)
  agent_minor_version_without_patch="${agent_minor_version%.*}"
fi

agent_dist_channel=stable
if [ -n "$DD_AGENT_DIST_CHANNEL" ]; then
  if [ "$repository_url" == "datadoghq.com" ]; then
    if [ "$DD_AGENT_DIST_CHANNEL" != "stable" ] && [ "$DD_AGENT_DIST_CHANNEL" != "beta" ]; then
      echo "DD_AGENT_DIST_CHANNEL must be either 'stable' or 'beta'. Current value: $DD_AGENT_DIST_CHANNEL"
      exit 1;
    fi
  elif [ "$DD_AGENT_DIST_CHANNEL" != "stable" ] && [ "$DD_AGENT_DIST_CHANNEL" != "beta" ] && [ "$DD_AGENT_DIST_CHANNEL" != "nightly" ]; then
    echo "DD_AGENT_DIST_CHANNEL must be either 'stable', 'beta' or 'nightly' on custom repos. Current value: $DD_AGENT_DIST_CHANNEL"
    exit 1;
  fi
  agent_dist_channel=$DD_AGENT_DIST_CHANNEL
fi

if [ -n "$TESTING_YUM_VERSION_PATH" ]; then
  yum_version_path=$TESTING_YUM_VERSION_PATH
else
  yum_version_path="${agent_dist_channel}/${agent_major_version}"
fi

if [ -n "$TESTING_APT_REPO_VERSION" ]; then
  apt_repo_version=$TESTING_APT_REPO_VERSION
else
  apt_repo_version="${agent_dist_channel} ${agent_major_version}"
fi

report_failure_url="https://api.datadoghq.com/agent_stats/report_failure"
if [ -n "$DD_SITE" ]; then
    report_failure_url="https://api.${DD_SITE}/agent_stats/report_failure"
fi
if [ -n "$TESTING_REPORT_URL" ]; then
  report_failure_url=$TESTING_REPORT_URL
fi

if [ ! "$apikey" ]; then
  # if it's an upgrade, then we will use the transition script
  if [ ! "$upgrade" ] && [ ! -e "$config_file" ]; then
    printf "\033[31mAPI key not available in DD_API_KEY environment variable.\033[0m\n"
    exit 1;
  fi
fi

if [[ `uname -m` == "armv7l" ]] && [[ $agent_flavor == "datadog-agent" ]]; then
    printf "\033[31mThe full $nice_flavor isn't available for your architecture (armv7l).\nInstall the ${flavor_to_readable[datadog-iot-agent]} by setting DD_AGENT_FLAVOR='datadog-iot-agent'.\033[0m\n"
    exit 1;
fi

# OS/Distro Detection
# Try lsb_release, fallback with /etc/issue then uname command
KNOWN_DISTRIBUTION="(Debian|Ubuntu|RedHat|CentOS|openSUSE|Amazon|Arista|SUSE|Rocky|AlmaLinux)"
DISTRIBUTION=$(lsb_release -d 2>/dev/null | grep -Eo $KNOWN_DISTRIBUTION  || grep -Eo $KNOWN_DISTRIBUTION /etc/issue 2>/dev/null || grep -Eo $KNOWN_DISTRIBUTION /etc/Eos-release 2>/dev/null || grep -m1 -Eo $KNOWN_DISTRIBUTION /etc/os-release 2>/dev/null || uname -s)

if [ "$DISTRIBUTION" = "Darwin" ]; then
    printf "\033[31mThis script does not support installing on the Mac.

Please use the 1-step script available at https://app.datadoghq.com/account/settings#agent/mac.\033[0m\n"
    exit 1;

elif [ -f /etc/debian_version ] || [ "$DISTRIBUTION" == "Debian" ] || [ "$DISTRIBUTION" == "Ubuntu" ]; then
    OS="Debian"
elif [ -f /etc/redhat-release ] || [ "$DISTRIBUTION" == "RedHat" ] || [ "$DISTRIBUTION" == "CentOS" ] || [ "$DISTRIBUTION" == "Amazon" ] || [ "$DISTRIBUTION" == "Rocky" ] || [ "$DISTRIBUTION" == "AlmaLinux" ]; then
    OS="RedHat"
# Some newer distros like Amazon may not have a redhat-release file
elif [ -f /etc/system-release ] || [ "$DISTRIBUTION" == "Amazon" ]; then
    OS="RedHat"
# Arista is based off of Fedora14/18 but do not have /etc/redhat-release
elif [ -f /etc/Eos-release ] || [ "$DISTRIBUTION" == "Arista" ]; then
    OS="RedHat"
# openSUSE and SUSE use /etc/SuSE-release or /etc/os-release
elif [ -f /etc/SuSE-release ] || [ "$DISTRIBUTION" == "SUSE" ] || [ "$DISTRIBUTION" == "openSUSE" ]; then
    OS="SUSE"
fi

if [[ "$agent_flavor" == "datadog-dogstatsd" ]]; then
    if [[ `uname -m` == "armv7l" ]] || { [[ `uname -m` != "x86_64" ]] && [[ "$OS" != "Debian" ]]; }; then
        printf "\033[31mThe $nice_flavor isn't available for your architecture.\033[0m\n"
        exit 1;
    fi
    if  [[ "$OS" == "Debian" ]] && [[ `uname -m` == "aarch64" ]] && { [[ -n "$agent_minor_version" ]] && [[ "$agent_minor_version" -lt 35 ]]; }; then
        printf "\033[31mThe $nice_flavor is only available since version 7.35.0 for your architecture.\033[0m\n"
        exit 1;
    fi
fi

# Root user detection
if [ "$(echo "$UID")" = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

# Install the necessary package sources
if [ "$OS" = "RedHat" ]; then
    remove_rpm_gpg_keys "$sudo_cmd" "${RPM_GPG_KEYS_TO_REMOVE[@]}"
    if { [ "$DISTRIBUTION" == "Rocky" ] || [ "$DISTRIBUTION" == "AlmaLinux" ]; } && { [ -n "$agent_minor_version" ] && [ "$agent_minor_version" -lt 33 ]; } && ! echo "$agent_flavor" | grep '[0-9]' > /dev/null; then
        echo -e "\033[33mA future version of $nice_flavor will support $DISTRIBUTION\n\033[0m"
        exit;
    fi
    echo -e "\033[34m\n* Installing YUM sources for Datadog\n\033[0m"

    UNAME_M=$(uname -m)
    if [ "$UNAME_M" == "i686" ] || [ "$UNAME_M" == "i386" ] || [ "$UNAME_M" == "x86" ]; then
        ARCHI="i386"
    elif [ "$UNAME_M" == "aarch64" ]; then
        ARCHI="aarch64"
    else
        ARCHI="x86_64"
    fi

    # Because of https://bugzilla.redhat.com/show_bug.cgi?id=1792506, we disable
    # repo_gpgcheck on RHEL/CentOS 8.1
    if [ -z "$rpm_repo_gpgcheck" ]; then
        if grep -q "8\.1\(\b\|\.\)" /etc/redhat-release 2>/dev/null; then
            rpm_repo_gpgcheck=0
        else
            rpm_repo_gpgcheck=1
        fi
    fi

    if [ "$agent_major_version" -eq 7 ]; then
      gpgkeys="https://${keys_url}/DATADOG_RPM_KEY_E09422B3.public"
    else
      gpgkeys="https://${keys_url}/DATADOG_RPM_KEY.public\n       https://${keys_url}/DATADOG_RPM_KEY_E09422B3.public"
    fi

    gpgkeys=''
    separator='\n       '
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      gpgkeys="${gpgkeys:+"${gpgkeys}${separator}"}https://${keys_url}/${key_path}"
    done

    $sudo_cmd sh -c "echo -e '[datadog]\nname = Datadog, Inc.\nbaseurl = https://${yum_url}/${yum_version_path}/${ARCHI}/\nenabled=1\ngpgcheck=1\nrepo_gpgcheck=${rpm_repo_gpgcheck}\npriority=1\ngpgkey=${gpgkeys}' > /etc/yum.repos.d/datadog.repo"

    printf "\033[34m* Installing the $nice_flavor package\n\033[0m\n"
    $sudo_cmd yum -y clean metadata

    dnf_flag=""
    if [ -f "/etc/fedora-release" ] && [ -f "/usr/bin/dnf" ]; then
      # On Fedora, yum is an alias of dnf, dnf install doesn't
      # upgrade a package if a newer version is available, unless
      # the --best flag is set
      dnf_flag="--best"
    fi

    if [ -n "$agent_minor_version" ]; then
        # Example: datadog-agent-7.20.2-1
        pkg_pattern="$agent_major_version\.${agent_minor_version%.}(\.[[:digit:]]+){0,1}(-[[:digit:]])?"
        agent_version_custom="$(yum -y --disablerepo=* --enablerepo=datadog list --showduplicates datadog-agent | sort -r | grep -E "$pkg_pattern" -om1)" || true
        verify_agent_version "-"
    fi

    declare -a packages
    packages=("$agent_flavor")
    if [ -n "$fips_mode" ]; then
      packages+=("datadog-fips-proxy")
    fi

    echo -e "  \033[33mInstalling package(s): ${packages[*]}\n\033[0m"

    $sudo_cmd yum -y --disablerepo='*' --enablerepo='datadog' install $dnf_flag "${packages[@]}" || $sudo_cmd yum -y install $dnf_flag "${packages[@]}"

elif [ "$OS" = "Debian" ]; then
    apt_trusted_d_keyring="/etc/apt/trusted.gpg.d/datadog-archive-keyring.gpg"
    apt_usr_share_keyring="/usr/share/keyrings/datadog-archive-keyring.gpg"

    printf "\033[34m\n* Installing apt-transport-https, curl and gnupg\n\033[0m\n"
    $sudo_cmd apt-get update || printf "\033[31m'apt-get update' failed, the script will not install the latest version of apt-transport-https.\033[0m\n"
    # installing curl might trigger install of additional version of libssl; this will fail the installation process,
    # see https://unix.stackexchange.com/q/146283 for reference - we use DEBIAN_FRONTEND=noninteractive to fix that
    if [ -z "$sudo_cmd" ]; then
        # if $sudo_cmd is empty, doing `$sudo_cmd X=Y command` fails with
        # `X=Y: command not found`; therefore we don't prefix the command with
        # $sudo_cmd at all in this case
        DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg
    else
        $sudo_cmd DEBIAN_FRONTEND=noninteractive apt-get install -y apt-transport-https curl gnupg
    fi
    printf "\033[34m\n* Installing APT package sources for Datadog\n\033[0m\n"
    $sudo_cmd sh -c "echo 'deb [signed-by=${apt_usr_share_keyring}] https://${apt_url}/ ${apt_repo_version}' > /etc/apt/sources.list.d/datadog.list"

    if [ ! -f $apt_usr_share_keyring ]; then
        $sudo_cmd touch $apt_usr_share_keyring
    fi
    # ensure that the _apt user used on Ubuntu/Debian systems to read GPG keyrings
    # can read our keyring
    $sudo_cmd chmod a+r $apt_usr_share_keyring

    for key in "${APT_GPG_KEYS[@]}"; do
        $sudo_cmd curl --retry 5 -o "/tmp/${key}" "https://${keys_url}/${key}"
        $sudo_cmd cat "/tmp/${key}" | $sudo_cmd gpg --import --batch --no-default-keyring --keyring "$apt_usr_share_keyring"
    done

    release_version="$(grep VERSION_ID /etc/os-release | cut -d = -f 2 | xargs echo | cut -d "." -f 1)"
    if { [ "$DISTRIBUTION" == "Debian" ] && [ "$release_version" -lt 9 ]; } || \
       { [ "$DISTRIBUTION" == "Ubuntu" ] && [ "$release_version" -lt 16 ]; }; then
        # copy with -a to preserve file permissions
        $sudo_cmd cp -a $apt_usr_share_keyring $apt_trusted_d_keyring
    fi

    if [ "$DISTRIBUTION" == "Debian" ] && [ "$release_version" -lt 8 ]; then
      if [ -n "$agent_minor_version_without_patch" ]; then
          if [ "$agent_minor_version_without_patch" -ge "36" ]; then
              printf "\033[31mDebian < 8 only supports $nice_flavor %s up to %s.35.\033[0m\n" "$agent_major_version" "$agent_major_version"
              exit;
          fi
      else
          if ! echo "$agent_flavor" | grep '[0-9]' > /dev/null; then
              echo -e "  \033[33m$nice_flavor $agent_major_version.35 is the last supported version on $DISTRIBUTION $release_version. Installing $agent_major_version.35 now.\n\033[0m"
              agent_minor_version=35
          fi
      fi
    fi

    printf "\033[34m\n* Installing the $nice_flavor package\n\033[0m\n"
    ERROR_MESSAGE="ERROR
Failed to update the sources after adding the Datadog repository.
This may be due to any of the configured APT sources failing -
see the logs above to determine the cause.
If the failing repository is Datadog, please contact Datadog support.
*****
"
    $sudo_cmd apt-get update -o Dir::Etc::sourcelist="sources.list.d/datadog.list" -o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0"
    ERROR_MESSAGE="ERROR
Failed to install the $agent_flavor package, sometimes it may be
due to another APT source failing. See the logs above to
determine the cause.
If the cause is unclear, please contact Datadog support.
*****
"

    if [ -n "$agent_minor_version" ]; then
        # Example: datadog-agent=1:7.20.2-1
        pkg_pattern="([[:digit:]]:)?$agent_major_version\.${agent_minor_version%.}(\.[[:digit:]]+){0,1}(-[[:digit:]])?"
        agent_version_custom="$(apt-cache madison datadog-agent | grep -E "$pkg_pattern" -om1)" || true
        verify_agent_version "="
    fi

    declare -a packages
    packages=("$agent_flavor" "datadog-signing-keys")
    if [ -n "$fips_mode" ]; then
     packages+=("datadog-fips-proxy")
    fi

    echo -e "  \033[33mInstalling package(s): ${packages[*]}\n\033[0m"

    $sudo_cmd apt-get install -y --force-yes "${packages[@]}"

    ERROR_MESSAGE=""
elif [ "$OS" = "SUSE" ]; then
  remove_rpm_gpg_keys "$sudo_cmd" "${RPM_GPG_KEYS_TO_REMOVE[@]}"
  UNAME_M=$(uname -m)
  if [ "$UNAME_M"  == "i686" ] || [ "$UNAME_M"  == "i386" ] || [ "$UNAME_M"  == "x86" ]; then
      printf "\033[31mThe Datadog Agent installer is only available for 64 bit SUSE Enterprise machines.\033[0m\n"
      exit;
  elif [ "$UNAME_M"  == "aarch64" ]; then
      ARCHI="aarch64"
  else
      ARCHI="x86_64"
  fi

  if [ -z "$rpm_repo_gpgcheck" ]; then
      rpm_repo_gpgcheck=1
  fi

  # Try to guess if we're installing on SUSE 11, as it needs a different flow to work
  # Note that SUSE11 doesn't have /etc/os-release file, so we have to use /etc/SuSE-release
  if cat /etc/SuSE-release 2>/dev/null | grep VERSION | grep 11; then
    SUSE11="yes"
  fi

  # Doing "rpm --import" requires curl on SUSE/SLES
  echo -e "\033[34m\n* Ensuring curl is installed\n\033[0m\n"
  if ! rpm -q curl > /dev/null; then
    # If zypper fails to refresh a random repo, it installs the package, but then fails
    # anyway. Therefore we let it do its thing and then see if curl was installed or not.
    if [ -z "$sudo_cmd" ]; then
      ZYPP_RPM_DEBUG="${ZYPP_RPM_DEBUG:-0}" zypper --non-interactive install curl ||:
    else
      $sudo_cmd ZYPP_RPM_DEBUG="${ZYPP_RPM_DEBUG:-0}" zypper --non-interactive install curl ||:
    fi
    if ! rpm -q curl > /dev/null; then
      echo -e "\033[31mFailed to install curl.\033[0m\n"
      fallback_msg
      exit 1;
    fi
  fi

  echo -e "\033[34m\n* Importing the Datadog GPG Keys\n\033[0m"
  if [ "$SUSE11" == "yes" ]; then
    # SUSE 11 special case
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      $sudo_cmd curl -o "/tmp/${key_path}" "https://${keys_url}/${key_path}"
      $sudo_cmd rpm --import "/tmp/${key_path}"
    done
  else
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      $sudo_cmd rpm --import "https://${keys_url}/${key_path}"
    done
  fi

  # Parse the major version number out of the distro release info file. xargs is used to trim whitespace.
  # NOTE: We use this to find out whether or not release version is >= 15, so we have to use /etc/os-release,
  # as /etc/SuSE-release has been deprecated and is no longer present everywhere, e.g. in AWS AMI.
  # See https://www.suse.com/releasenotes/x86_64/SUSE-SLES/15/#fate-324409
  SUSE_VER=$(cat /etc/os-release 2>/dev/null | grep VERSION_ID | tr -d '"' | tr . = | cut -d = -f 2 | xargs echo)
  gpgkeys="https://${keys_url}/DATADOG_RPM_KEY_CURRENT.public"
  if [ -n "$SUSE_VER" ] && [ "$SUSE_VER" -ge 15 ] && [ "$SUSE_VER" -ne 42 ]; then
    gpgkeys=''
    separator='\n       '
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      gpgkeys="${gpgkeys:+"${gpgkeys}${separator}"}https://${keys_url}/${key_path}"
    done
  fi

  echo -e "\033[34m\n* Installing YUM Repository for Datadog\n\033[0m"
  $sudo_cmd sh -c "echo -e '[datadog]\nname=datadog\nenabled=1\nbaseurl=https://${yum_url}/suse/${yum_version_path}/${ARCHI}\ntype=rpm-md\ngpgcheck=1\nrepo_gpgcheck=${rpm_repo_gpgcheck}\ngpgkey=${gpgkeys}' > /etc/zypp/repos.d/datadog.repo"

  echo -e "\033[34m\n* Refreshing repositories\n\033[0m"
  $sudo_cmd zypper --non-interactive --no-gpg-checks refresh datadog

  echo -e "\033[34m\n* Installing the $nice_flavor package\n\033[0m"

  # ".32" is the latest version supported for OpenSUSE < 15 and SLES < 12
  # we explicitly test for SUSE11 = "yes", as some SUSE11 don't have /etc/os-release, thus SUSE_VER is empty
  if [ "$DISTRIBUTION" == "openSUSE" ] && { [ "$SUSE11" == "yes" ] || [ "$SUSE_VER" -lt 15 ]; }; then
      if [ -n "$agent_minor_version_without_patch" ]; then
          if [ "$agent_minor_version_without_patch" -ge "33" ]; then
              printf "\033[31mopenSUSE < 15 only supports $nice_flavor %s up to %s.32.\033[0m\n" "$agent_major_version" "$agent_major_version"
              exit;
          fi
      else
          if ! echo "$agent_flavor" | grep '[0-9]' > /dev/null; then
              echo -e "  \033[33m$nice_flavor $agent_major_version.32 is the last supported version on $DISTRIBUTION $SUSE_VER\n\033[0m"
              agent_minor_version=32
          fi
      fi
  fi
  if [ "$DISTRIBUTION" == "SUSE" ] && { [ "$SUSE11" == "yes" ] || [ "$SUSE_VER" -lt 12 ]; }; then
      if [ -n "$agent_minor_version_without_patch" ]; then
          if [ "$agent_minor_version_without_patch" -ge "33" ]; then
              printf "\033[31mSLES < 12 only supports $nice_flavor %s up to %s.32.\033[0m\n" "$agent_major_version" "$agent_major_version"
              exit;
          fi
      else
          if ! echo "$agent_flavor" | grep '[0-9]' > /dev/null; then
              echo -e "  \033[33m$nice_flavor $agent_major_version.32 is the last supported version on $DISTRIBUTION $SUSE_VER\n\033[0m"
              agent_minor_version=32
          fi
      fi
  fi

  if [ -n "$agent_minor_version" ]; then
      # Example: datadog-agent-1:7.20.2-1
      pkg_pattern="([[:digit:]]:)?$agent_major_version\.${agent_minor_version%.}(\.[[:digit:]]+){0,1}(-[[:digit:]])?"
      agent_version_custom="$(zypper search -s datadog-agent | grep -E "$pkg_pattern" -om1)" || true
      verify_agent_version "-"
  fi

  declare -a packages
  packages=("$agent_flavor")
  if [ -n "$fips_mode" ]; then
    packages+=("datadog-fips-proxy")
  fi


  echo -e "  \033[33mInstalling package(s): ${packages[*]}\n\033[0m"

  if [ -z "$sudo_cmd" ]; then
    ZYPP_RPM_DEBUG="${ZYPP_RPM_DEBUG:-0}" zypper --non-interactive install "${packages[@]}" ||:
  else
    $sudo_cmd ZYPP_RPM_DEBUG="${ZYPP_RPM_DEBUG:-0}" zypper --non-interactive install "${packages[@]}" ||:
  fi

  # If zypper fails to refresh a random repo, it installs the package, but then fails
  # anyway. Therefore we let it do its thing and then see if curl was installed or not.
  for expected_pkg in "${packages[@]}"; do
    if ! rpm -q "${expected_pkg}" > /dev/null; then
      echo -e "\033[31mFailed to install ${expected_pkg}.\033[0m\n"
      fallback_msg
      exit 1;
    fi
  done

else
    printf "\033[31mYour OS or distribution are not supported by this install script.
Please follow the instructions on the Agent setup page:

    https://app.datadoghq.com/account/settings#agent\033[0m\n"
    exit;
fi

if [ "$upgrade" ] && [ "$agent_flavor" != "datadog-dogstatsd" ]; then
  if [ -e $LEGACY_CONF ]; then
    # try to import the config file from the previous version
    icmd="datadog-agent import $LEGACY_ETCDIR $etcdir"
    # shellcheck disable=SC2086
    $sudo_cmd $icmd || printf "\033[31mAutomatic import failed, you can still try to manually run: $icmd\n\033[0m\n"
    # fix file owner and permissions since the script moves around some files
    $sudo_cmd chown -R dd-agent:dd-agent "$etcdir"
    $sudo_cmd find "$etcdir/" -type f -exec chmod 640 {} \;
  else
    printf "\033[31mYou don't have a datadog.conf file to convert.\n\033[0m\n"
  fi
fi

# Set the configuration
if [ -e "$config_file" ] && [ -z "$upgrade" ]; then
  printf "\033[34m\n* Keeping old $config_file configuration file\n\033[0m\n"
else
  if [ ! -e "$config_file" ]; then
    $sudo_cmd cp "$config_file.example" "$config_file"
  fi
  if [ "$apikey" ]; then
    printf "\033[34m\n* Adding your API key to the $nice_flavor configuration: $config_file\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/api_key:.*/api_key: $apikey/' $config_file"
  else
    # If the import script failed for any reason, we might end here also in case
    # of upgrade, let's not start the agent or it would fail because the api key
    # is missing
    if ! $sudo_cmd grep -q -E '^api_key: .+' "$config_file"; then
      printf "\033[31mThe $nice_flavor won't start automatically at the end of the script because the Api key is missing, please add one in datadog.yaml and start the $nice_flavor manually.\n\033[0m\n"
      no_start=true
    fi
  fi

  if [ -z "$fips_mode" ]; then
    if [ "$site" ]; then
      printf "\033[34m\n* Setting SITE in the $nice_flavor configuration: $config_file\n\033[0m\n"
      $sudo_cmd sh -c "sed -i 's/# site:.*/site: $site/' $config_file"
    fi
    if [ -n "$DD_URL" ]; then
      printf "\033[34m\n* Setting DD_URL in the $nice_flavor configuration: $config_file\n\033[0m\n"
      $sudo_cmd sh -c "sed -i 's|# dd_url:.*|dd_url: $DD_URL|' $config_file"
    fi
  else
    printf "\033[34m\n* Setting $nice_flavor configuration to use FIPS proxy: $config_file\n\033[0m\n"
    $sudo_cmd cp "$config_file" "${config_file}.orig"
    $sudo_cmd sh -c "exec cat - '${config_file}.orig' > '$config_file'" <<EOF
# Configuration for the agent to use datadog-fips-proxy to communicate with Datadog via FIPS-compliant channel.

dd_url: http://localhost:9804

apm_config:
    apm_dd_url: http://localhost:9805
    profiling_dd_url: http://localhost:9806
    telemetry:
        dd_url: http://localhost:9813

process_config:
    process_dd_url: http://localhost:9807

logs_config:
    use_http: true
    logs_dd_url: localhost:9808
    logs_no_ssl: true

database_monitoring:
    metrics:
        dd_url: localhost:9809
    activity:
        dd_url: localhost:9809
    samples:
        dd_url: localhost:9810

network_devices:
    metadata:
        dd_url: localhost:9811
EOF
  fi
  if [ "$hostname" ]; then
    printf "\033[34m\n* Adding your HOSTNAME to the $nice_flavor configuration: $config_file\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/# hostname:.*/hostname: $hostname/' $config_file"
  fi
  if [ "$host_tags" ]; then
      printf "\033[34m\n* Adding your HOST TAGS to the $nice_flavor configuration: $config_file\n\033[0m\n"
      formatted_host_tags="['""$( echo "$host_tags" | sed "s/,/','/g" )""']"  # format `env:prod,foo:bar` to yaml-compliant `['env:prod','foo:bar']`
      $sudo_cmd sh -c "sed -i \"s/# tags:.*/tags: ""$formatted_host_tags""/\" $config_file"
  fi
fi

$sudo_cmd chown dd-agent:dd-agent "$config_file"
$sudo_cmd chmod 640 "$config_file"

# set the FIPS configuration
if [ -n "$fips_mode" ]; then
  if [ -e "$config_file_fips" ] && [ -z "$upgrade" ]; then
    printf "\033[34m\n* Keeping old $config_file_fips configuration file\n\033[0m\n"
  else
    if [ ! -e "$config_file_fips" ]; then
      $sudo_cmd cp "$config_file_fips.example" "$config_file_fips"

      # TODO: set port range in file, or environment variable
    fi

    $sudo_cmd chown dd-agent:dd-agent "$config_file_fips"
    $sudo_cmd chmod 640 "$config_file_fips"

  fi
fi

# Creating or overriding the install information
install_info_content="---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-$install_script_version
"
$sudo_cmd sh -c "echo '$install_info_content' > $etcdir/install_info"

if [ -n "$fips_mode" ]; then
  # Creating or overriding the install information
  $sudo_cmd sh -c "echo '$install_info_content' > $etcdirfips/install_info"
fi

# On SUSE 11, sudo service datadog-agent start fails (because /sbin is not in a base user's path)
# However, sudo /sbin/service datadog-agent does work.
# Use which (from root user) to find the absolute path to service

service_cmd="service"
if [ "$SUSE11" == "yes" ]; then
  service_cmd=`$sudo_cmd which service`
fi

declare -a monitoring_services
monitoring_services=( "datadog-agent" )

if [ $no_start ]; then
  printf "\033[34m\n  * DD_INSTALL_ONLY environment variable set.\033[0m\n"
fi

for current_service in "${services[@]}"; do
  nice_current_flavor=${flavor_to_readable[$current_service]}

  # Use /usr/sbin/service by default.
  # Some distros usually include compatibility scripts with Upstart or Systemd. Check with: `command -v service | xargs grep -E "(upstart|systemd)"`
  restart_cmd="$sudo_cmd $service_cmd $current_service restart"
  stop_instructions="$sudo_cmd $service_cmd $current_service stop"
  start_instructions="$sudo_cmd $service_cmd $current_service start"

  if [[ `$sudo_cmd ps --no-headers -o comm 1 2>&1` == "systemd" ]] && command -v systemctl 2>&1; then
    # Use systemd if systemctl binary exists and systemd is the init process
    restart_cmd="$sudo_cmd systemctl restart ${current_service}.service"
    stop_instructions="$sudo_cmd systemctl stop $current_service"
    start_instructions="$sudo_cmd systemctl start $current_service"
  elif /sbin/init --version 2>&1 | grep -q upstart; then
    # Try to detect Upstart, this works most of the times but still a best effort
    restart_cmd="$sudo_cmd stop $current_service || true ; sleep 2s ; $sudo_cmd start $current_service"
    stop_instructions="$sudo_cmd stop $current_service"
    start_instructions="$sudo_cmd start $current_service"
  fi

  if [ $no_start ]; then
    printf "\033[34m\n    The newly installed version of the ${nice_current_flavor} will not be started.
    You will have to do it manually using the following command:

    $start_instructions\033[0m\n\n"

    continue
  fi

  printf "\033[34m* Starting the ${nice_current_flavor}...\n\033[0m\n"
  eval "$restart_cmd"


  # Metrics are submitted, echo some instructions and exit
  printf "\033[32m  Your ${nice_current_flavor} is running and functioning properly.\n\033[0m"

  if [[ "${monitoring_services[*]}" =~ ${current_service} ]]; then
    printf "\033[32m  It will continue to run in the background and submit metrics to Datadog.\n\033[0m"
  fi

  printf "\033[32m  If you ever want to stop the ${nice_current_flavor}, run:

      $stop_instructions

  And to run it again run:

      $start_instructions\033[0m\n\n"
done
