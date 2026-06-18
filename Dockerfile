FROM ubuntu:24.04

RUN apt-get update && apt install -y ca-certificates

RUN DEBIAN_FRONTEND=noninteractive TZ=Etc/UTC apt-get -y install tzdata

COPY simple-discord-bot /app/
CMD ["/app/simple-discord-bot", "--config", "/config/config.yaml"]
