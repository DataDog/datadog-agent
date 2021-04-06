#!/bin/bash
# (C) Datadog, Inc. 2010-2016
# All rights reserved
# Licensed under Simplified BSD License (see LICENSE)
# Datadog Agent installation script: install and set up the Agent on supported Linux distributions
# using the package manager and Datadog repositories.

set -e
install_script_version=1.3.1
logfile="ddagent-install.log"
support_email=support@datadoghq.com

LEGACY_ETCDIR="/etc/dd-agent"
LEGACY_CONF="$LEGACY_ETCDIR/datadog.conf"
ETCDIR="/etc/datadog-agent"
CONF="$ETCDIR/datadog.yaml"

# A2923DFF56EDA6E76E55E492D3A80E30382E94DE expires in 2022
# D75CEA17048B9ACBF186794B32637D44F14F620E expires in 2032
APT_GPG_KEYS=("A2923DFF56EDA6E76E55E492D3A80E30382E94DE" "D75CEA17048B9ACBF186794B32637D44F14F620E")

# DATADOG_RPM_KEY_CURRENT.public always contains key used to sign current
# repodata and newly released packages
# DATADOG_RPM_KEY_E09422B3.public expires in 2022
# DATADOG_RPM_KEY_FD4BF915.public expires in 2024
RPM_GPG_KEYS=("DATADOG_RPM_KEY_CURRENT.public" "DATADOG_RPM_KEY_E09422B3.public" "DATADOG_RPM_KEY_FD4BF915.public")

# RPM_GPG_KEYS_A6 contains keys we only install for the A6 repo.
# DATADOG_RPM_KEY.public is only useful to install old (< 6.14) Agent packages.
RPM_GPG_KEYS_A6=("DATADOG_RPM_KEY.public")

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
    fallback_msg
  fi
}

function on_read_error() {
  printf "Timed out or input EOF reached, assuming 'No'\n"
  yn="n"
}

function on_error() {
    printf "\033[31m$ERROR_MESSAGE
It looks like you hit an issue when trying to install the Agent.

Troubleshooting and basic usage information for the Agent are available at:

    https://docs.datadoghq.com/agent/basic_agent_usage/\n\033[0m\n"

    if ! tty -s; then
      fallback_msg
      exit 1;
    fi
    
    while true; do
        read -t 60 -p  "Do you want to send a failure report to Datadog (including $logfile)? (y/[n]) " -r yn || on_read_error
        case $yn in
          [Yy]* )
            read -p "Enter an email address so we can follow up: " -r email
            report
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

if [ -n "$TESTING_APT_URL" ]; then
  apt_url=$TESTING_APT_URL
else
  apt_url="apt.${repository_url}"
fi

upgrade=
if [ -n "$DD_UPGRADE" ]; then
  upgrade=$DD_UPGRADE
fi

agent_major_version=6
if [ -n "$DD_AGENT_MAJOR_VERSION" ]; then
  if [ "$DD_AGENT_MAJOR_VERSION" != "6" ] && [ "$DD_AGENT_MAJOR_VERSION" != "7" ]; then
    echo "DD_AGENT_MAJOR_VERSION must be either 6 or 7. Current value: $DD_AGENT_MAJOR_VERSION"
    exit 1;
  fi
  agent_major_version=$DD_AGENT_MAJOR_VERSION
else
  echo -e "\033[33mWarning: DD_AGENT_MAJOR_VERSION not set. Installing Agent version 6 by default.\033[0m"
fi

agent_flavor="datadog-agent"
if [ -n "$DD_AGENT_FLAVOR" ]; then
    agent_flavor=$DD_AGENT_FLAVOR #Eg: datadog-iot-agent
fi

agent_dist_channel=stable
if [ -n "$DD_AGENT_DIST_CHANNEL" ]; then
  if [ "$DD_AGENT_DIST_CHANNEL" != "stable" ] && [ "$DD_AGENT_DIST_CHANNEL" != "beta" ]; then
    echo "DD_AGENT_DIST_CHANNEL must be either 'stable' or 'beta'. Current value: $DD_AGENT_DIST_CHANNEL"
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

keyserver="hkp://keyserver.ubuntu.com:80"
backup_keyserver="hkp://pool.sks-keyservers.net:80"
# use this env var to specify another key server, such as
# hkp://p80.pool.sks-keyservers.net:80 for example.
if [ -n "$DD_KEYSERVER" ]; then
  keyserver="$DD_KEYSERVER"
fi

report_failure_url="https://api.datadoghq.com/agent_stats/report_failure"
if [ -n "$TESTING_REPORT_URL" ]; then
  report_failure_url=$TESTING_REPORT_URL
fi

if [ ! "$apikey" ]; then
  # if it's an upgrade, then we will use the transition script
  if [ ! "$upgrade" ]; then
    printf "\033[31mAPI key not available in DD_API_KEY environment variable.\033[0m\n"
    exit 1;
  fi
fi

# OS/Distro Detection
# Try lsb_release, fallback with /etc/issue then uname command
KNOWN_DISTRIBUTION="(Debian|Ubuntu|RedHat|CentOS|openSUSE|Amazon|Arista|SUSE)"
DISTRIBUTION=$(lsb_release -d 2>/dev/null | grep -Eo $KNOWN_DISTRIBUTION  || grep -Eo $KNOWN_DISTRIBUTION /etc/issue 2>/dev/null || grep -Eo $KNOWN_DISTRIBUTION /etc/Eos-release 2>/dev/null || grep -m1 -Eo $KNOWN_DISTRIBUTION /etc/os-release 2>/dev/null || uname -s)

if [ "$DISTRIBUTION" = "Darwin" ]; then
    printf "\033[31mThis script does not support installing on the Mac.

Please use the 1-step script available at https://app.datadoghq.com/account/settings#agent/mac.\033[0m\n"
    exit 1;

elif [ -f /etc/debian_version ] || [ "$DISTRIBUTION" == "Debian" ] || [ "$DISTRIBUTION" == "Ubuntu" ]; then
    OS="Debian"
elif [ -f /etc/redhat-release ] || [ "$DISTRIBUTION" == "RedHat" ] || [ "$DISTRIBUTION" == "CentOS" ] || [ "$DISTRIBUTION" == "Amazon" ]; then
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

# Root user detection
if [ "$(echo "$UID")" = "0" ]; then
    sudo_cmd=''
else
    sudo_cmd='sudo'
fi

# Install the necessary package sources
if [ "$OS" = "RedHat" ]; then
    echo -e "\033[34m\n* Installing YUM sources for Datadog\n\033[0m"

    UNAME_M=$(uname -m)
    if [ "$UNAME_M" == "i686" ] || [ "$UNAME_M" == "i386" ] || [ "$UNAME_M" == "x86" ]; then
        ARCHI="i386"
    elif [ "$UNAME_M" == "aarch64" ]; then
        ARCHI="aarch64"
    else
        ARCHI="x86_64"
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
    if [ "$agent_major_version" -eq 6 ]; then
      for key_path in "${RPM_GPG_KEYS_A6[@]}"; do
        gpgkeys="${gpgkeys:+"${gpgkeys}${separator}"}https://${keys_url}/${key_path}"
      done
    fi

    $sudo_cmd sh -c "echo -e '[datadog]\nname = Datadog, Inc.\nbaseurl = https://${yum_url}/${yum_version_path}/${ARCHI}/\nenabled=1\ngpgcheck=1\nrepo_gpgcheck=0\npriority=1\ngpgkey=${gpgkeys}' > /etc/yum.repos.d/datadog.repo"

    printf "\033[34m* Installing the Datadog Agent package\n\033[0m\n"
    $sudo_cmd yum -y clean metadata

    dnf_flag=""
    if [ -f "/etc/fedora-release" ] && [ -f "/usr/bin/dnf" ]; then
      # On Fedora, yum is an alias of dnf, dnf install doesn't
      # upgrade a package if a newer version is available, unless
      # the --best flag is set
      dnf_flag="--best"
    fi

    $sudo_cmd yum -y --disablerepo='*' --enablerepo='datadog' install $dnf_flag "$agent_flavor" || $sudo_cmd yum -y install $dnf_flag "$agent_flavor"

elif [ "$OS" = "Debian" ]; then

    printf "\033[34m\n* Installing apt-transport-https\n\033[0m\n"
    $sudo_cmd apt-get update || printf "\033[31m'apt-get update' failed, the script will not install the latest version of apt-transport-https.\033[0m\n"
    $sudo_cmd apt-get install -y apt-transport-https
    # Only install dirmngr if it's available in the cache
    # it may not be available on Ubuntu <= 14.04 but it's not required there
    cache_output=`apt-cache search dirmngr`
    if [ ! -z "$cache_output" ]; then
      $sudo_cmd apt-get install -y dirmngr
    fi
    printf "\033[34m\n* Installing APT package sources for Datadog\n\033[0m\n"
    $sudo_cmd sh -c "echo 'deb https://${apt_url}/ ${apt_repo_version}' > /etc/apt/sources.list.d/datadog.list"

    for key in "${APT_GPG_KEYS[@]}"; do
      for retries in {0..4}; do
        $sudo_cmd apt-key adv --recv-keys --keyserver "${keyserver}" "${key}" && break
        if [ "$retries" -eq 4 ]; then
          ERROR_MESSAGE="ERROR
  Couldn't fetch Datadog public key ${key}.
  This might be due to a connectivity error with the key server
  or a temporary service interruption.
  *****
  "
          false
        fi
        printf "\033[33m\napt-key failed to retrieve Datadog's public key ${key}, retrying in 5 seconds...\n\033[0m\n"
        sleep 5
        if [ "$retries" -eq 1 ]; then
          printf "\033[34mSwitching to backup keyserver\n\033[0m\n"
          keyserver="${backup_keyserver}"
        fi
      done
    done

    printf "\033[34m\n* Installing the Datadog Agent package\n\033[0m\n"
    ERROR_MESSAGE="ERROR
Failed to update the sources after adding the Datadog repository.
This may be due to any of the configured APT sources failing -
see the logs above to determine the cause.
If the failing repository is Datadog, please contact Datadog support.
*****
"
    $sudo_cmd apt-get update -o Dir::Etc::sourcelist="sources.list.d/datadog.list" -o Dir::Etc::sourceparts="-" -o APT::Get::List-Cleanup="0"
    ERROR_MESSAGE="ERROR
Failed to install the Datadog package, sometimes it may be
due to another APT source failing. See the logs above to
determine the cause.
If the cause is unclear, please contact Datadog support.
*****
"
    $sudo_cmd apt-get install -y --force-yes "$agent_flavor"
    ERROR_MESSAGE=""
elif [ "$OS" = "SUSE" ]; then
  UNAME_M=$(uname -m)
  if [ "$UNAME_M"  == "i686" ] || [ "$UNAME_M"  == "i386" ] || [ "$UNAME_M"  == "x86" ]; then
      printf "\033[31mThe Datadog Agent installer is only available for 64 bit SUSE Enterprise machines.\033[0m\n"
      exit;
  elif [ "$UNAME_M"  == "aarch64" ]; then
      ARCHI="aarch64"
  else
      ARCHI="x86_64"
  fi

  # Try to guess if we're installing on SUSE 11, as it needs a different flow to work
  if cat /etc/SuSE-release 2>/dev/null | grep VERSION | grep 11; then
    SUSE11="yes"
  fi

  echo -e "\033[34m\n* Importing the Datadog GPG Keys\n\033[0m"
  if [ "$SUSE11" == "yes" ]; then
    # SUSE 11 special case
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      $sudo_cmd curl -o "/tmp/${key_path}" "https://${keys_url}/${key_path}"
      $sudo_cmd rpm --import "/tmp/${key_path}"
    done
    if [ "$agent_major_version" -eq 6 ]; then
      for key_path in "${RPM_GPG_KEYS_A6[@]}"; do
        $sudo_cmd curl -o "/tmp/${key_path}" "https://${keys_url}/${key_path}"
        $sudo_cmd rpm --import "/tmp/${key_path}"
      done
    fi
  else
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      $sudo_cmd rpm --import "https://${keys_url}/${key_path}"
    done
    if [ "$agent_major_version" -eq 6 ]; then
      for key_path in "${RPM_GPG_KEYS_A6[@]}"; do
        $sudo_cmd rpm --import "https://${keys_url}/${key_path}"
      done
    fi
  fi

  # parse the major version number out of the distro release info file. xargs is used to trim whitespace.
  SUSE_VER=$( (cat /etc/SuSE-release 2>/dev/null; cat /etc/SUSE-brand 2>/dev/null) | grep VERSION | tr . = | cut -d = -f 2 | xargs echo)
  if [ "$SUSE_VER" -ge 15 ]; then
    gpgkeys=''
    separator='\n       '
    for key_path in "${RPM_GPG_KEYS[@]}"; do
      gpgkeys="${gpgkeys:+"${gpgkeys}${separator}"}https://${keys_url}/${key_path}"
    done
    if [ "$agent_major_version" -eq 6 ]; then
      for key_path in "${RPM_GPG_KEYS_A6[@]}"; do
        gpgkeys="${gpgkeys:+"${gpgkeys}${separator}"}https://${keys_url}/${key_path}"
      done
    fi
  else
    gpgkeys="https://${keys_url}/DATADOG_RPM_KEY_CURRENT.public"
  fi

  echo -e "\033[34m\n* Installing YUM Repository for Datadog\n\033[0m"
  $sudo_cmd sh -c "echo -e '[datadog]\nname=datadog\nenabled=1\nbaseurl=https://${yum_url}/suse/${yum_version_path}/${ARCHI}\ntype=rpm-md\ngpgcheck=1\nrepo_gpgcheck=0\ngpgkey=${gpgkeys}' > /etc/zypp/repos.d/datadog.repo"

  echo -e "\033[34m\n* Refreshing repositories\n\033[0m"
  $sudo_cmd zypper --non-interactive --no-gpg-checks refresh datadog

  echo -e "\033[34m\n* Installing Datadog Agent\n\033[0m"
  $sudo_cmd zypper --non-interactive install "$agent_flavor"

else
    printf "\033[31mYour OS or distribution are not supported by this install script.
Please follow the instructions on the Agent setup page:

    https://app.datadoghq.com/account/settings#agent\033[0m\n"
    exit;
fi

if [ "$upgrade" ]; then
  if [ -e $LEGACY_CONF ]; then
    # try to import the config file from the previous version
    icmd="datadog-agent import $LEGACY_ETCDIR $ETCDIR"
    # shellcheck disable=SC2086
    $sudo_cmd $icmd || printf "\033[31mAutomatic import failed, you can still try to manually run: $icmd\n\033[0m\n"
    # fix file owner and permissions since the script moves around some files
    $sudo_cmd chown -R dd-agent:dd-agent $ETCDIR
    $sudo_cmd find $ETCDIR/ -type f -exec chmod 640 {} \;
  else
    printf "\033[31mYou don't have a datadog.conf file to convert.\n\033[0m\n"
  fi
fi

# Set the configuration
if [ -e $CONF ] && [ -z "$upgrade" ]; then
  printf "\033[34m\n* Keeping old datadog.yaml configuration file\n\033[0m\n"
else
  if [ ! -e $CONF ]; then
    $sudo_cmd cp $CONF.example $CONF
  fi
  if [ "$apikey" ]; then
    printf "\033[34m\n* Adding your API key to the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/api_key:.*/api_key: $apikey/' $CONF"
  else
    # If the import script failed for any reason, we might end here also in case
    # of upgrade, let's not start the agent or it would fail because the api key
    # is missing
    if ! $sudo_cmd grep -q -E '^api_key: .+' $CONF; then
      printf "\033[31mThe Agent won't start automatically at the end of the script because the Api key is missing, please add one in datadog.yaml and start the agent manually.\n\033[0m\n"
      no_start=true
    fi
  fi
  if [ "$site" ]; then
    printf "\033[34m\n* Setting SITE in the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/# site:.*/site: $site/' $CONF"
  fi
  if [ -n "$DD_URL" ]; then
    printf "\033[34m\n* Setting DD_URL in the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's|# dd_url:.*|dd_url: $DD_URL|' $CONF"
  fi
  if [ "$hostname" ]; then
    printf "\033[34m\n* Adding your HOSTNAME to the Agent configuration: $CONF\n\033[0m\n"
    $sudo_cmd sh -c "sed -i 's/# hostname:.*/hostname: $hostname/' $CONF"
  fi
  if [ "$host_tags" ]; then
      printf "\033[34m\n* Adding your HOST TAGS to the Agent configuration: $CONF\n\033[0m\n"
      formatted_host_tags="['""$( echo "$host_tags" | sed "s/,/','/g" )""']"  # format `env:prod,foo:bar` to yaml-compliant `['env:prod','foo:bar']`
      $sudo_cmd sh -c "sed -i \"s/# tags:.*/tags: ""$formatted_host_tags""/\" $CONF"
  fi
  $sudo_cmd chown dd-agent:dd-agent $CONF
  $sudo_cmd chmod 640 $CONF
fi

# Creating or overriding the install information
install_info_content="---
install_method:
  tool: install_script
  tool_version: install_script
  installer_version: install_script-$install_script_version
"
$sudo_cmd sh -c "echo '$install_info_content' > $ETCDIR/install_info"

# On SUSE 11, sudo service datadog-agent start fails (because /sbin is not in a base user's path)
# However, sudo /sbin/service datadog-agent does work.
# Use which (from root user) to find the absolute path to service

service_cmd="service"
if [ "$SUSE11" == "yes" ]; then
  service_cmd=`$sudo_cmd which service`
fi

# Use /usr/sbin/service by default.
# Some distros usually include compatibility scripts with Upstart or Systemd. Check with: `command -v service | xargs grep -E "(upstart|systemd)"`
restart_cmd="$sudo_cmd $service_cmd datadog-agent restart"
stop_instructions="$sudo_cmd $service_cmd datadog-agent stop"
start_instructions="$sudo_cmd $service_cmd datadog-agent start"

if command -v systemctl 2>&1; then
  # Use systemd if systemctl binary exists
  restart_cmd="$sudo_cmd systemctl restart datadog-agent.service"
  stop_instructions="$sudo_cmd systemctl stop datadog-agent"
  start_instructions="$sudo_cmd systemctl start datadog-agent"
elif /sbin/init --version 2>&1 | grep -q upstart; then
  # Try to detect Upstart, this works most of the times but still a best effort
  restart_cmd="$sudo_cmd stop datadog-agent || true ; sleep 2s ; $sudo_cmd start datadog-agent"
  stop_instructions="$sudo_cmd stop datadog-agent"
  start_instructions="$sudo_cmd start datadog-agent"
fi

if [ $no_start ]; then
    printf "\033[34m
* DD_INSTALL_ONLY environment variable set: the newly installed version of the agent
will not be started. You will have to do it manually using the following
command:

    $start_instructions

\033[0m\n"
    exit
fi

printf "\033[34m* Starting the Agent...\n\033[0m\n"
eval "$restart_cmd"


# Metrics are submitted, echo some instructions and exit
printf "\033[32m

Your Agent is running and functioning properly. It will continue to run in the
background and submit metrics to Datadog.

If you ever want to stop the Agent, run:

    $stop_instructions

And to run it again run:

    $start_instructions

\033[0m"
