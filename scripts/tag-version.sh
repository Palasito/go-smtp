#!/bin/bash
# Test basic SMTP connection and send a message.
# The relay must either have this host's IP whitelisted (no AUTH needed)
# or you must set USERNAME and PASSWORD env vars for plain-auth.
#
# Usage: ./scripts/test-smtp.sh [host] [port]
#
# Examples:
#   ./scripts/test-smtp.sh                         # localhost:8025, whitelisted IP
#   USERNAME=user PASSWORD=pass ./scripts/test-smtp.sh 10.0.0.5 587

Tag=${1}

git checkout main
git pull
git tag $Tag
git push origin $Tag