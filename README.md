# Nyaa.si Scraper

## Usage guide

1. Fill out $count in `generate-docker-compose.sh`, $count should include how many scrapers you want running

2. Run the script (`./generate-docker-compose.sh`)

It will generate a docker compose file for you into `generated-docker-compose.yml`, rename it to `docker-compose.yml`. Make sure to fill out gluetun fields (its usually VPN credentials). Consult [gluetun documentation](https://github.com/qdm12/gluetun) for further info.

If you dont want to use vpn, which you probably should be using run the script with ``--novpn`` flag (`./generate-docker-compose.sh --novpn`)


## Build docker container
`nix run .#docker.copyToDockerDaemon`
