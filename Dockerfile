FROM alpine:latest

RUN apk add --no-cache tzdata

RUN apk add --update-cache \
  tzdata \
  bash \
  && rm -rf /var/cache/apk/*

COPY simple-discord-bot /app/
CMD ["/app/simple-discord-bot", "--config", "/config/config.yaml"]
