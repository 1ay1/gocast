#!/bin/bash

# Test source script for debugging GoCast streaming
# This script provides several ways to test source streaming

set -e

GOCAST_HOST="${GOCAST_HOST:-localhost}"
GOCAST_PORT="${GOCAST_PORT:-8001}"
MOUNT_PATH="${MOUNT_PATH:-/live}"
SOURCE_USER="${SOURCE_USER:-source}"
SOURCE_PASS="${SOURCE_PASS:-hackme}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}GoCast Test Source Script${NC}"
echo "========================================"
echo "Host: $GOCAST_HOST:$GOCAST_PORT"
echo "Mount: $MOUNT_PATH"
echo ""

usage() {
    echo "Usage: $0 <command>"
    echo ""
    echo "Commands:"
    echo "  ffmpeg-mp3     Stream MP3 audio using FFmpeg (sine wave)"
    echo "  ffmpeg-file    Stream an MP3 file using FFmpeg"
    echo "  curl-raw       Send raw bytes using curl (for basic connectivity test)"
    echo "  netcat         Send continuous raw bytes using netcat"
    echo "  check          Check server connectivity"
    echo ""
    echo "Environment variables:"
    echo "  GOCAST_HOST    Server host (default: localhost)"
    echo "  GOCAST_PORT    Server port (default: 8001)"
    echo "  MOUNT_PATH     Mount path (default: /live)"
    echo "  SOURCE_USER    Source username (default: source)"
    echo "  SOURCE_PASS    Source password (default: hackme)"
    echo "  MP3_FILE       Path to MP3 file (for ffmpeg-file command)"
}

# Check if server is reachable
check_server() {
    echo -e "${YELLOW}Checking server connectivity...${NC}"
    if curl -s -o /dev/null -w "%{http_code}" "http://$GOCAST_HOST:$GOCAST_PORT/" | grep -q "200\|404"; then
        echo -e "${GREEN}✓ Server is reachable${NC}"
        return 0
    else
        echo -e "${RED}✗ Server is not reachable${NC}"
        return 1
    fi
}

# FFmpeg streaming with sine wave (no file needed)
stream_ffmpeg_mp3() {
    echo -e "${YELLOW}Starting FFmpeg MP3 stream (sine wave)...${NC}"
    echo "Press Ctrl+C to stop"
    echo ""

    # Use -re for real-time streaming
    # Generate a 440Hz sine wave and encode as MP3
    ffmpeg -re -f lavfi -i "sine=frequency=440:duration=3600" \
        -c:a libmp3lame -b:a 128k -f mp3 \
        -content_type audio/mpeg \
        -ice_name "Test Stream" \
        -ice_description "GoCast Test Stream" \
        -ice_genre "Test" \
        "icecast://${SOURCE_USER}:${SOURCE_PASS}@${GOCAST_HOST}:${GOCAST_PORT}${MOUNT_PATH}" \
        2>&1 | while read line; do
            echo "[FFmpeg] $line"
        done
}

# FFmpeg streaming from file
stream_ffmpeg_file() {
    if [ -z "$MP3_FILE" ]; then
        echo -e "${RED}Error: MP3_FILE environment variable not set${NC}"
        echo "Usage: MP3_FILE=/path/to/file.mp3 $0 ffmpeg-file"
        exit 1
    fi

    if [ ! -f "$MP3_FILE" ]; then
        echo -e "${RED}Error: File not found: $MP3_FILE${NC}"
        exit 1
    fi

    echo -e "${YELLOW}Starting FFmpeg stream from file: $MP3_FILE${NC}"
    echo "Press Ctrl+C to stop"
    echo ""

    # -stream_loop -1 for infinite looping
    ffmpeg -re -stream_loop -1 -i "$MP3_FILE" \
        -c:a libmp3lame -b:a 128k -f mp3 \
        -content_type audio/mpeg \
        -ice_name "Test Stream" \
        "icecast://${SOURCE_USER}:${SOURCE_PASS}@${GOCAST_HOST}:${GOCAST_PORT}${MOUNT_PATH}" \
        2>&1 | while read line; do
            echo "[FFmpeg] $line"
        done
}

# Raw curl test (sends some bytes to test basic connectivity)
test_curl_raw() {
    echo -e "${YELLOW}Testing with curl (PUT method)...${NC}"

    # Create a small test file with random bytes
    TEMP_FILE=$(mktemp)
    dd if=/dev/urandom of="$TEMP_FILE" bs=1024 count=10 2>/dev/null

    echo "Sending 10KB of random data..."

    curl -v -X PUT \
        -u "${SOURCE_USER}:${SOURCE_PASS}" \
        -H "Content-Type: audio/mpeg" \
        -H "Ice-Name: Test Stream" \
        --data-binary "@$TEMP_FILE" \
        "http://${GOCAST_HOST}:${GOCAST_PORT}${MOUNT_PATH}"

    rm -f "$TEMP_FILE"
    echo ""
    echo -e "${GREEN}Done${NC}"
}

# Netcat continuous stream (for low-level debugging)
test_netcat() {
    echo -e "${YELLOW}Starting netcat continuous stream...${NC}"
    echo "This sends raw bytes with minimal HTTP handshake"
    echo "Press Ctrl+C to stop"
    echo ""

    # Base64 encode credentials
    AUTH=$(echo -n "${SOURCE_USER}:${SOURCE_PASS}" | base64)

    # Create a FIFO for continuous streaming
    FIFO=$(mktemp -u)
    mkfifo "$FIFO"

    # Cleanup on exit
    trap "rm -f $FIFO" EXIT

    # Start the HTTP request and continuous data in background
    {
        # Send HTTP headers
        echo -e "PUT ${MOUNT_PATH} HTTP/1.1\r"
        echo -e "Host: ${GOCAST_HOST}:${GOCAST_PORT}\r"
        echo -e "Authorization: Basic ${AUTH}\r"
        echo -e "Content-Type: audio/mpeg\r"
        echo -e "Transfer-Encoding: chunked\r"
        echo -e "Ice-Name: Netcat Test\r"
        echo -e "\r"

        # Continuously send random data
        while true; do
            dd if=/dev/urandom bs=1024 count=1 2>/dev/null
            sleep 0.1
        done
    } | nc "$GOCAST_HOST" "$GOCAST_PORT"
}

# Alternative: simple SOURCE method test (Icecast-style)
test_source_method() {
    echo -e "${YELLOW}Testing with SOURCE method (Icecast-style)...${NC}"
    echo "Press Ctrl+C to stop"
    echo ""

    AUTH=$(echo -n "${SOURCE_USER}:${SOURCE_PASS}" | base64)

    {
        echo -e "SOURCE ${MOUNT_PATH} HTTP/1.0\r"
        echo -e "Host: ${GOCAST_HOST}:${GOCAST_PORT}\r"
        echo -e "Authorization: Basic ${AUTH}\r"
        echo -e "Content-Type: audio/mpeg\r"
        echo -e "Ice-Name: Test Stream\r"
        echo -e "\r"

        # Send random data continuously
        while true; do
            dd if=/dev/urandom bs=1024 count=1 2>/dev/null
            sleep 0.1
        done
    } | nc "$GOCAST_HOST" "$GOCAST_PORT"
}

case "${1:-}" in
    ffmpeg-mp3)
        check_server && stream_ffmpeg_mp3
        ;;
    ffmpeg-file)
        check_server && stream_ffmpeg_file
        ;;
    curl-raw)
        check_server && test_curl_raw
        ;;
    netcat)
        check_server && test_netcat
        ;;
    source)
        check_server && test_source_method
        ;;
    check)
        check_server
        ;;
    *)
        usage
        exit 1
        ;;
esac
