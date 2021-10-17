FROM alpine:latest

ADD simple-discord-bot /app/
CMD ["/app/simple-discord-bot", "--config", "/config/config.yaml"]
