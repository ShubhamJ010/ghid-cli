#!/bin/bash
# GhostHID Legacy CLI Wrapper
# Redirects transparently to the standalone 'ghid' CLI engine.
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$DIR/ghid" "$@"
