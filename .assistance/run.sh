. ./env

if [[ -z $1 ]]; then
  echo "Please specify a command, (run, provision, destroy)"
  exit 1
fi

if [[ $1 == "provision" ]]; then
  vagrant provision --provision-with "$2"

elif [[ $1 == "destroy" ]]; then
  vagrant -f destroy && vagrant up

else
  vagrant destroy
  vagrant up
fi
