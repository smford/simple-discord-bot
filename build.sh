#!/usr/bin/env bash
set -eux
MYAPP="simple-discord-bot"

# Check if the first parameter is provided
if [ -z "${1:-}" ]; then
  echo "Error: REGISTRY_USERNAME is not provided."
  echo "Usage: $0 <REGISTRY_USERNAME>"
  exit 1
fi

# Set the REGISTRY_USERNAME variable
REGISTRY_USERNAME="$1"

# Get the current date and time in the format yyyy-mm-dd hh:mm:ss
current_time=$(date +"%Y-%m-%d %H:%M:%S")

# Replace the buildDateTime value in main.go with the current time
sed -i "s|const buildDateTime string = \".*\"|const buildDateTime string = \"${current_time}\"|g" main.go

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -v ./...
upx ./${MYAPP}

VERSION=$(cat main.go|grep ^const\ applic|cut -f5 -d\ |sed 's/\"//g')

export DOCKER_DEFAULT_PLATFORM=linux/amd64
exit
die
docker build -t ${REGISTRY_USERNAME}/${MYAPP}:${VERSION} -t ${REGISTRY_USERNAME}/${MYAPP}:latest .
docker push ${REGISTRY_USERNAME}/${MYAPP}:${VERSION}
docker push ${REGISTRY_USERNAME}/${MYAPP}
