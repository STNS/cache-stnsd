#!/bin/sh

use_systemctl="True"
systemd_version=0
if ! command -V systemctl >/dev/null 2>&1; then
  use_systemctl="False"
else
  systemd_version=$(systemctl --version | head -1 | sed 's/systemd //g')
fi

remove() {
    printf "\033[32m Post Remove of a normal remove\033[0m\n"

    if [ "${use_systemctl}" = "False" ]; then
        if command -V chkconfig >/dev/null 2>&1; then
            chkconfig --del cache-stnsd
        fi
        service cache-stnsd stop ||:
    else
        printf "\033[32m Reload the service unit from disk\033[0m\n"
        systemctl daemon-reload ||:
        printf "\033[32m Disable the service\033[0m\n"
        systemctl disable cache-stnsd ||:
        printf "\033[32m Stop the service\033[0m\n"
        systemctl stop cache-stnsd ||:
        printf "\033[32m Mask the service\033[0m\n"
        systemctl mask cache-stnsd ||:
    fi
}

echo "$@"

action="$1"

case "$action" in
  "0" | "remove")
    remove
    ;;
  "1" | "upgrade")
    ;;
  "purge")
    remove
    ;;
esac
