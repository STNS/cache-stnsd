#!/bin/sh

use_systemctl="True"
systemd_version=0
if ! command -V systemctl >/dev/null 2>&1; then
  use_systemctl="False"
else
    systemd_version=$(systemctl --version | head -1 | sed 's/systemd //g')
fi

upgrade() {
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    if [ "${use_systemctl}" = "False" ]; then
        if command -V chkconfig >/dev/null 2>&1; then
          chkconfig --add cache-stnsd
        fi
        service cache-stnsd restart ||:
    else
        printf "\033[32m Reload the service unit from disk\033[0m\n"
        systemctl daemon-reload ||:
        printf "\033[32m Unmask the service\033[0m\n"
        systemctl unmask cache-stnsd ||:
        printf "\033[32m Set the preset flag for the service unit\033[0m\n"
        systemctl preset cache-stnsd ||:
        printf "\033[32m Set the enabled flag for the service unit\033[0m\n"
        systemctl enable cache-stnsd ||:
        systemctl restart cache-stnsd ||:
    fi

}

case "$action" in
  "2" | "upgrade")
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    upgrade
    ;;
esac
