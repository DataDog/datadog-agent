#!/bin/bash

function resolveLatest() {
	git tag --sort=-taggerdate | head -1
}

function _hasGitChanges() {
	test -n "$(git status -s)"
}

function tagExists() {
	tag=${1:-$(resolveLatest)}
	test -n "$tag" && test -n "$(git tag | grep "^$tag\$")"
}

function differsFromLatest() {
  tag=$(resolveLatest)
  headtag=$(git tag -l --points-at HEAD)
  if tagExists $tag; then
    if [ "$tag" == "$headtag" ]; then
      #[I] tag $tag exists, and matches tag for the commit
      return 1
    else
      #[I] Codebase differs: $tag does not match commit.
      return 0
    fi
  else
    # [I] No tag found for $tag
    return 0
  fi
}

function getVersion() {
	result=$(resolveLatest)

	if differsFromLatest; then
		result="$result-$(git rev-parse --short HEAD)"
	fi

	if _hasGitChanges ; then
		result="$result-raw"
	fi
	echo $result
}

getVersion
