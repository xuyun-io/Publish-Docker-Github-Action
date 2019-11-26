#!/bin/sh
set -e

function main() {
  echo "" # see https://github.com/actions/toolkit/issues/168

  sanitize "${INPUT_NAME}" "name"
  sanitize "${INPUT_USERNAME}" "username"
  sanitize "${INPUT_PASSWORD}" "password"

  translateDockerTag
  DOCKERNAME="${INPUT_NAME}:${TAG}"

  if uses "${INPUT_WORKDIR}"; then
    changeWorkingDirectory
  fi

  echo ${INPUT_PASSWORD} | docker login -u ${INPUT_USERNAME} --password-stdin ${INPUT_REGISTRY}

  BUILDPARAMS=""

  if uses "${INPUT_DOCKERFILE}"; then
    useCustomDockerfile
  fi
  if uses "${INPUT_BUILDARGS}"; then
    addBuildArgs
  fi
  if uses "${INPUT_CACHE}"; then
    useBuildCache
  fi

  if uses "${INPUT_SNAPSHOT}"; then
    pushWithSnapshot
  else
    pushWithoutSnapshot
  fi
  echo ::set-output name=tag::"${TAG}"

  docker logout
}

function sanitize() {
  if [ -z "${1}" ]; then
    >&2 echo "Unable to find the ${2}. Did you set with.${2}?"
    exit 1
  fi
}

function translateDockerTag() {
  local BRANCH=$(echo ${GITHUB_REF} | sed -e "s/refs\/heads\///g" | sed -e "s/\//-/g")
  if hasCustomTag; then
    TAG=$(echo ${INPUT_NAME} | cut -d':' -f2)
    INPUT_NAME=$(echo ${INPUT_NAME} | cut -d':' -f1)
  elif isOnMaster; then
#     TAG=$(date +%Y%m%d%H%M%S)-$(echo ${GITHUB_SHA} | cut -c1-8)
     TAG="latest"
  elif isOnDev; then
     TAG="latest"
  elif isGitTag; then
    TAG="latest"
  elif isPullRequest; then
    TAG="${GITHUB_SHA}"
  else
    TAG=$(date +%Y%m%d%H%M%S)-$(echo ${GITHUB_SHA} | cut -c1-8)-${BRANCH}
  fi;
}

function hasCustomTag() {
  [ $(echo ${INPUT_NAME} | sed -e "s/://g") != ${INPUT_NAME} ]
}

function isOnMaster() {
  [ "${BRANCH}" = "master" ]
}

function isOnDev() {
  [ "${BRANCH}" = "develop" ]
}

function isGitTag() {
  [ $(echo ${GITHUB_REF} | sed -e "s/refs\/tags\///g") != ${GITHUB_REF} ]
}

function isPullRequest() {
  [ $(echo ${GITHUB_REF} | sed -e "s/refs\/pull\///g") != ${GITHUB_REF} ]
}

function changeWorkingDirectory() {
  cd "${INPUT_WORKDIR}"
}

function useCustomDockerfile() {
  BUILDPARAMS="$BUILDPARAMS -f ${INPUT_DOCKERFILE}"
}

function addBuildArgs() {
  for arg in $(echo "${INPUT_BUILDARGS}" | tr ',' '\n'); do
    BUILDPARAMS="$BUILDPARAMS --build-arg ${arg}"
    echo "::add-mask::${arg}"
  done
}

function useBuildCache() {
  if docker pull ${DOCKERNAME} 2>/dev/null; then
    BUILDPARAMS="$BUILDPARAMS --cache-from ${DOCKERNAME}"
  fi
}

function uses() {
  if [ ! -z "${1}" ]; then
    return 0
  else
    return 1
  fi
}

function pushWithSnapshot() {
  local TIMESTAMP=`date +%Y%m%d%H%M%S`
  local SHORT_SHA=$(echo "${GITHUB_SHA}" | cut -c1-6)
  local SNAPSHOT_TAG="${TIMESTAMP}${SHORT_SHA}"
  local SHA_DOCKER_NAME="${INPUT_NAME}:${SNAPSHOT_TAG}"
  pwd && ls && ls -l 
  docker build $BUILDPARAMS -t ${DOCKERNAME} -t ${SHA_DOCKER_NAME} .
  docker push ${DOCKERNAME}
  docker push ${SHA_DOCKER_NAME}
  echo ::set-output name=snapshot-tag::"${SNAPSHOT_TAG}"
}

function pushWithoutSnapshot() {
  docker build $BUILDPARAMS -t ${DOCKERNAME} .
  docker push ${DOCKERNAME}
}

main
