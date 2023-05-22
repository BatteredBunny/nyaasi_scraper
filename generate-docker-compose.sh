#!/usr/bin/env nix-shell
#!nix-shell -i fish -p fish bat yq

# Config
# -----
set count 5 # how many scrapers you want running
set request_length_ms 700 # Used for calculating estimated completion time
# -----

if test $(curl "https://nyaa.si/?page=rss" -I | head -n 1 | cut -d ' ' -f2) = 200
  set max $(curl "https://nyaa.si/?page=rss" | xq ".rss.channel.item[0].link" | sed -e "s/https:\/\/nyaa.si\/download\///" -e "s/[.]torrent//" | tr -d '"')
else
  set max 1673894 # default value when actual latest one cant be fetched
end

set slice $(math floor $max / $count)
set estimated_hours $(math $max / $count x $request_length_ms / 1000 / 60 / 60)
argparse 'novpn' -- $argv

# docker compose header
set compose "version: \"3\""\n"services:"\n

# gluetun entries
if test not $_flag_novpn
  for i in $(seq $count)
      set -a compose "\
  gluetun$i:
    image: qmcgaw/gluetun
    container_name: gluetun$i
    cap_add:
      - NET_ADMIN
    devices:
      - /dev/net/tun:/dev/net/tun
    environment:
      - VPN_SERVICE_PROVIDER=mullvad
      - VPN_TYPE=wireguard
      - SERVER_CITIES=Dusseldorf
      - WIREGUARD_PRIVATE_KEY=
      - WIREGUARD_ADDRESSES=
      - TZ=Europe/Tallinn"\n
  end
end

# scraper entries
for i in $(seq $count)
    set end $(math $i x $slice)
    set start $(math $end - $slice+1)

    set -a compose "\
  nyaasi_scraper$i:
    image: ghcr.io/ayes-web/nyaasi_scraper
    container_name: nyaasi_scraper$i"\n

    if test not $_flag_novpn
      set -a compose "   network_mode: service:gluetun$i"\n
    end

    set -a compose "\
   command: --start $start --end $end --continue-in-range
    volumes:
      - ./app:/app/"\n
end

set tmp $(mktemp)
echo Estimated completion time: $estimated_hours hours | cat > $tmp
echo $compose | cat > generated-docker-compose.yml

bat $tmp generated-docker-compose.yml
rm $tmp