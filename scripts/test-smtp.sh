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

HOST=${1:-localhost}
PORT=${2:-8025}
FROM=${FROM:-testmdmcommit@mdm.commitandpush.com}
TO=${TO:-palas@spworxx.com}

echo "Testing SMTP connection to $HOST:$PORT..."

# Build the full SMTP dialogue using printf so every line ends with CRLF (\r\n)
# as required by RFC 5321.  A trailing sleep keeps stdin open long enough for
# the server to send its final response before nc exits and closes the socket.
{
    printf "EHLO test\r\n";            sleep 1

    if [[ -n "$USERNAME" && -n "$PASSWORD" ]]; then
        # AUTH PLAIN — base64("\0username\0password")
        CREDS=$(printf '\0%s\0%s' "$USERNAME" "$PASSWORD" | base64)
        printf "AUTH PLAIN %s\r\n" "$CREDS"; sleep 1
    fi

    printf "MAIL FROM:<%s>\r\n" "$FROM"; sleep 1
    printf "RCPT TO:<%s>\r\n"   "$TO";   sleep 1
    printf "DATA\r\n";                   sleep 1

    # Message headers + blank line + body + DATA terminator
    printf "From: %s\r\n"        "$FROM"
    printf "To: %s\r\n"          "$TO"
    printf "Subject: Automated Test Email\r\n"
    printf "\r\n"
    printf "This is a test email sent from the go-smtp relay.\r\n"
    printf ".\r\n";                      sleep 2

    printf "QUIT\r\n";                   sleep 2
} | nc -w 10 "$HOST" "$PORT"

echo ""
echo "Test complete. Check the output above for SMTP response codes."
echo "Expected: 220 greeting, 250 after EHLO/MAIL/RCPT, 354 after DATA, 250 after '.', 221 after QUIT."
