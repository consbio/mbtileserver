#!/bin/bash

# Adapted from:
# https://sosedoff.com/2019/02/12/go-github-actions.html
# https://github.com/ngs/go-release.action/blob/master/entrypoint.sh

set -e

# Build targets
# Omit: darwin/amd64 darwin/386   (MacOS requires signed, notarized binaries now)
# Omit: windows/amd64 windows/386  (CGO cross-compile for Windows is more work)
targets=${@-"linux/amd64 linux/386 linux/arm64"}


# Get repo information from the github event
EVENT_DATA=$(cat $GITHUB_EVENT_PATH)
# echo $EVENT_DATA | jq .
UPLOAD_URL=$(echo $EVENT_DATA | jq -r .release.upload_url)
UPLOAD_URL=${UPLOAD_URL/\{?name,label\}/}
RELEASE_NAME=$(echo $EVENT_DATA | jq -r .release.tag_name)

# Validate token.
curl -o /dev/null -sH "Authorization: token $GITHUB_TOKEN" "https://api.github.com/repos/$GITHUB_REPOSITORY" || { echo "Error: Invalid token or network issue!";  exit 1; }

tag=$(basename $GITHUB_REF)

if [[ -z "$GITHUB_WORKSPACE" ]]; then
  echo "Set the GITHUB_WORKSPACE env variable."
  exit 1
fi

if [[ -z "$GITHUB_REPOSITORY" ]]; then
  echo "Set the GITHUB_REPOSITORY env variable."
  exit 1
fi

root_path="/go/src/github.com/$GITHUB_REPOSITORY"
release_path="$GITHUB_WORKSPACE/.release"
repo_name="$(echo $GITHUB_REPOSITORY | cut -d '/' -f2)"


echo "----> Setting up Go repository"
mkdir -p $release_path
mkdir -p $root_path
cp -a $GITHUB_WORKSPACE/* $root_path/
cd $root_path

gcc=""

for target in $targets; do
  os="$(echo $target | cut -d '/' -f1)"
  arch="$(echo $target | cut -d '/' -f2)"

  output="${release_path}/${repo_name}_${tag}_${os}_${arch}"

  # install GCC for Arm64
  if [ $target == "linux/arm64" ]; then
    apt-get install -y gcc-aarch64-linux-gnu
    gcc="CC=/usr/bin/aarch64-linux-gnu-gcc"
  fi

  echo "----> Building project for: $target"
  GOOS=$os GOARCH=$arch CGO_ENABLED=1 $gcc go build -o "$output${ext}"
  zip -j $output.zip "$output${ext}" > /dev/null
done

echo "----> Build is complete. List of files at $release_path:"
cd $release_path
ls -al


# Upload to github release assets
for asset in "${release_path}"/*.zip; do
  file_name="$(basename "$asset")"

  status_code="$(curl -sS  -X POST \
    --write-out "%{http_code}" -o "/tmp/$file_name.json" \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Content-Length: $(stat -c %s "$asset")" \
    -H "Content-Type: application/zip" \
    --upload-file "$asset" \
    "$UPLOAD_URL?name=$file_name")"

  if [ "$status_code" -ne "201" ]; then
    >&2 printf "\n\tERR: Failed asset upload: %s\n" "$file_name"
    >&2 jq . < "/tmp/$file_name.json"
    exit 1
  fi
done

echo "----> Upload is complete"