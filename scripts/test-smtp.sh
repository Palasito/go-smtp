#!/bin/bash
# Test basic SMTP connection and EHLO
# Usage: ./scripts/test-smtp.sh [host] [port]
HOST=${1:-localhost}
PORT=${2:-8025}

echo "Testing SMTP connection to $HOST:$PORT..."

# Test 1: Basic connection and EHLO
(echo "EHLO test"; sleep 1; echo "QUIT") | nc -w 5 $HOST $PORT

echo ""
echo "Test complete. Check the output above for the SMTP greeting and EHLO response."
