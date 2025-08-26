# API Contract for vi-tg

## Current Endpoints

### Authentication
- `GET /api/auth/status` - Get authentication status
- `POST /api/auth/phone` - Set phone number
- `POST /api/auth/code` - Send authentication code

### Chats and Messages
- `GET /api/chats` - Get list of chats
- `GET /api/chats/{chat_id}/messages` - Get messages from chat
- `POST /api/chats/{chat_id}/messages` - Send message

### Media
- `GET /api/stickers/{sticker_id}` - Download sticker
- `GET /api/images/{image_id}` - Download image
- `GET /api/videos/{video_id}` - Download video
- `GET /api/voices/{voice_id}` - Download voice message
- `GET /api/audios/{audio_id}` - Download audio message

## New Endpoints for Location Support

### Location
- `GET /api/locations/{location_id}` - Get location data and static map
- `GET /api/locations/{location_id}/map` - Get map image for location

## Message Types

### Existing Types
- text
- photo
- video
- voice
- audio
- sticker

### New Types
- location - Location messages with coordinates
- venue - Location with venue information (name, address)
- live_location - Live location sharing

## Location Message Structure

```json
{
  "id": 12345,
  "text": "",
  "from": "John Doe",
  "timestamp": "2025-08-26T12:15:55+03:00",
  "chat_id": 67890,
  "type": "location",
  "location_id": 12345,
  "location_lat": 55.7558,
  "location_lng": 37.6173,
  "location_title": "Red Square",
  "location_address": "Red Square, Moscow, Russia"
}
```

## Location Data Structure

```json
{
  "id": 12345,
  "latitude": 55.7558,
  "longitude": 37.6173,
  "title": "Red Square",
  "address": "Red Square, Moscow, Russia",
  "map_image_path": "/tmp/vi-tg_location_map_12345.png"
}
```

## Implementation Plan

### Phase 1: Backend Changes
1. Add location fields to Message structure in Go backend
2. Update message parsing to handle location messages from Telegram API
3. Add location endpoints for serving map images
4. Implement static map generation (using external map service or library)

### Phase 2: Frontend Changes
1. Add location fields to Message struct in Rust
2. Update message display logic to show location messages
3. Add map display functionality in TUI
4. Handle location message interactions (open maps, etc.)

### Phase 3: Integration
1. Test location message receiving
2. Test location message display
3. Test map image generation and display
4. Handle edge cases (no map service, network errors, etc.)
