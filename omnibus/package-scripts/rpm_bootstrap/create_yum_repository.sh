#!/bin/sh

REPO_BUCKET_NAME=${STS_AWS_TEST_BUCKET_YUM:-stackstate-agent-rpm-test}
REPO_BASE=/tmp/repo_base
RPM_ARTIFACTS_PATH=${CI_PROJECT_DIR:-.}/outcomes/pkg
CODENAME=${CI_COMMIT_REF_NAME:-master}

rm -rf $REPO_BASE
mkdir -p $REPO_BASE/$CODENAME
echo cp $RPM_ARTIFACTS_PATH/*.rpm $REPO_BASE/$CODENAME
cp $RPM_ARTIFACTS_PATH/*.rpm $REPO_BASE/$CODENAME
createrepo $REPO_BASE

aws s3 sync $REPO_BASE s3://${REPO_BUCKET_NAME} --delete
