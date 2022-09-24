#!/usr/bin/env bash
set -eux
MYAPP="simple-discord-bot"
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w"
upx ./${MYAPP}

VERSION=$(cat ${MYAPP}.go|grep ^const\ applic|cut -f5 -d\ |sed 's/\"//g')

export DOCKER_DEFAULT_PLATFORM=linux/amd64

docker build -t smford/${MYAPP}:${VERSION} -t smford/${MYAPP}:latest .
docker push smford/${MYAPP}:${VERSION}
docker push smford/${MYAPP}
