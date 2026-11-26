#!/usr/bin/env bash
# Wrapper ngắn gọn: sudo bash install.sh --host doh.x.com --ip 1.2.3.4 --email you@mail.com
exec bash "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/scripts/install.sh" "$@"
