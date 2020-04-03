## python artifactory dependency
mkdir ~/.pip/ && touch ~/.pip/pip.conf
echo "[global]" > ~/.pip/pip.conf
echo "extra-index-url = https://$ARTIFACTORY_USER:$ARTIFACTORY_PASSWORD@$ARTIFACTORY_URL/simple" >> ~/.pip/pip.conf
