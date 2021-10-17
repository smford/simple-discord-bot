#!/usr/bin/env bash
set -eux
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w"
upx ./simple-discord-bot

VERSION=$(cat simple-discord-bot.go|grep ^const\ applic|cut -f5 -d\ |sed 's/\"//g')

docker build -t smford/simple-discord-bot:${VERSION} -t smford/simple-discord-bot:latest .
docker push smford/simple-discord-bot:${VERSION}
docker push smford/simple-discord-bot
