#!/bin/bash

# Script to generate video preview (first frame) for vi-tg
# Usage: ./generate_video_preview.sh <video_file_path> <video_id>

if [ $# -ne 2 ]; then
    echo "Usage: $0 <video_file_path> <video_id>"
    exit 1
fi

VIDEO_FILE="$1"
VIDEO_ID="$2"
PREVIEW_DIR="/tmp"
PREVIEW_FILE="$PREVIEW_DIR/vi-tg_video_preview_${VIDEO_ID}.jpg"

# Check if video file exists
if [ ! -f "$VIDEO_FILE" ]; then
    echo "Error: Video file '$VIDEO_FILE' not found"
    exit 1
fi

# Check if ffmpeg is available
if ! command -v ffmpeg &> /dev/null; then
    echo "Error: ffmpeg is not installed"
    exit 1
fi

# Extract first frame as preview with better quality settings
ffmpeg -i "$VIDEO_FILE" -ss 00:00:01 -vframes 1 -q:v 2 -vf "scale=720:-1:force_original_aspect_ratio=decrease" "$PREVIEW_FILE" 2>/dev/null

if [ $? -eq 0 ]; then
    echo "Preview generated: $PREVIEW_FILE"
    exit 0
else
    echo "Error: Failed to generate preview for $VIDEO_FILE"
    exit 1
fi
