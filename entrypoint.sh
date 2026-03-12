#!/bin/sh
set -e

# Write secrets from env vars to files (Coolify injects these as env vars).
if [ -n "$MASTER_KEY" ]; then
  printf '%s' "$MASTER_KEY" > /etc/openclaw-proxy/master_key
fi

if [ -n "$ADMIN_TOKEN" ]; then
  printf '%s' "$ADMIN_TOKEN" > /etc/openclaw-proxy/admin_token
fi

exec clawring
