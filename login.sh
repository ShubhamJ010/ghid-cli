#!/bin/bash
# GhostHID Legacy Login Wrapper
# Redirects transparently to 'ghid login'.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$DIR/ghid" login "$@"
