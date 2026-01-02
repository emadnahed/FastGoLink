# FastGoLink API Reference

This document provides a detailed reference for all FastGoLink API endpoints.

## Base URL

```
http://localhost:8080
```

For production, replace with your deployed URL.

## Authentication

Currently, FastGoLink does not require authentication. However, rate limiting is applied based on:
- Client IP address
- Optional `X-API-Key` header (for separate rate limit buckets)

## Rate Limiting

All endpoints are subject to rate limiting:

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests per window |
| `X-RateLimit-Remaining` | Remaining requests in current window |
| `X-RateLimit-Reset` | Unix timestamp when window resets |

When rate limited, you'll receive:
- Status: `429 Too Many Requests`
- Header: `Retry-After: <seconds>`

## Error Responses

All errors follow a consistent format:

```json
{
  "error": "Human-readable error message",
  "code": "MACHINE_READABLE_CODE"
}
```

### Error Codes

| Code | HTTP Status | Error Message | Description |
|------|-------------|---------------|-------------|
| `INVALID_REQUEST` | 400 | `invalid request body` | Malformed JSON request body |
| `INVALID_EXPIRES_IN` | 400 | `invalid expires_in duration format` | Invalid duration format for expires_in |
| `EMPTY_URL` | 400 | `url cannot be empty` | URL field is missing or empty |
| `INVALID_URL` | 400 | `invalid url format` | URL format is invalid |
| `INVALID_SHORT_CODE` | 400 | `short code is required` | Short code is missing in analytics request |
| `DANGEROUS_URL` | 400 | `URL contains dangerous scheme` | URL uses dangerous scheme (javascript:, data:, vbscript:, file:) |
| `PRIVATE_IP_BLOCKED` | 400 | `private IP addresses are not allowed` | URL points to private/local IP address |
| `BLOCKED_HOST` | 400 | `host is blocked` | URL host is in the configured blocklist |
| `URL_TOO_LONG` | 400 | `URL exceeds maximum length` | URL exceeds 2048 characters (configurable) |
| `NOT_FOUND` | 404 | `url not found` / `URL not found` | Short code does not exist |
| `EXPIRED` | 410 | `url has expired` | URL has passed its expiration time |
| `RETRY_EXCEEDED` | 503 | `service temporarily unavailable` | Short code generation failed after max retries |
| `RATE_LIMITED` | 429 | `rate limit exceeded` | Rate limit exceeded |
| `INTERNAL_ERROR` | 500 | `internal server error` | Internal server error |

---

## Endpoints

### Create Short URL

Creates a new shortened URL.

```
POST /api/v1/shorten
```

#### Request Body

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | The original URL to shorten |
| `expires_in` | string | No | Duration until expiration (e.g., "1h", "24h", "7d") |

#### Example Request

```bash
curl -X POST http://localhost:8080/api/v1/shorten \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/very/long/path?with=query&params=true",
    "expires_in": "24h"
  }'
```

#### Response (201 Created)

```json
{
  "short_url": "http://localhost:8080/abc1234",
  "short_code": "abc1234",
  "original_url": "https://example.com/very/long/path?with=query&params=true",
  "created_at": "2024-01-02T10:30:45Z",
  "expires_at": "2024-01-03T10:30:45Z"
}
```

#### Error Responses

| Status | Code | Error Message |
|--------|------|---------------|
| 400 | `INVALID_REQUEST` | `invalid request body` |
| 400 | `INVALID_EXPIRES_IN` | `invalid expires_in duration format` |
| 400 | `EMPTY_URL` | `url cannot be empty` |
| 400 | `INVALID_URL` | `invalid url format` |
| 400 | `DANGEROUS_URL` | `URL contains dangerous scheme` |
| 400 | `PRIVATE_IP_BLOCKED` | `private IP addresses are not allowed` |
| 400 | `BLOCKED_HOST` | `host is blocked` |
| 400 | `URL_TOO_LONG` | `URL exceeds maximum length` |
| 429 | `RATE_LIMITED` | `rate limit exceeded` |
| 503 | `RETRY_EXCEEDED` | `service temporarily unavailable` |

---

### Get URL Information

Retrieves information about a shortened URL.

```
GET /api/v1/urls/{code}
```

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `code` | string | The short code |

#### Example Request

```bash
curl http://localhost:8080/api/v1/urls/abc1234
```

#### Response (200 OK)

```json
{
  "short_code": "abc1234",
  "original_url": "https://example.com/very/long/path",
  "created_at": "2024-01-02T10:30:45Z",
  "expires_at": "2024-01-03T10:30:45Z",
  "click_count": 1523
}
```

#### Error Responses

| Status | Code | Error Message |
|--------|------|---------------|
| 404 | `NOT_FOUND` | `url not found` |
| 410 | `EXPIRED` | `url has expired` |

---

### Delete Short URL

Permanently deletes a shortened URL.

```
DELETE /api/v1/urls/{code}
```

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `code` | string | The short code |

#### Example Request

```bash
curl -X DELETE http://localhost:8080/api/v1/urls/abc1234
```

#### Response (204 No Content)

Empty response body on success.

#### Error Responses

| Status | Code | Error Message |
|--------|------|---------------|
| 404 | `NOT_FOUND` | `url not found` |

---

### Redirect

Redirects to the original URL.

```
GET /{code}
```

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `code` | string | The short code |

#### Example Request

```bash
curl -L http://localhost:8080/abc1234
```

#### Response

| Status | Description |
|--------|-------------|
| 302 | Temporary redirect to original URL |
| 301 | Permanent redirect (if configured) |
| 404 | Short code not found |
| 410 | URL has expired |

The `Location` header contains the original URL.

---

### Get Analytics

Retrieves click statistics for a shortened URL.

```
GET /api/v1/analytics/{code}
```

#### Path Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `code` | string | The short code |

#### Example Request

```bash
curl http://localhost:8080/api/v1/analytics/abc1234
```

#### Response (200 OK)

```json
{
  "short_code": "abc1234",
  "click_count": 1523,
  "pending_count": 12
}
```

| Field | Description |
|-------|-------------|
| `click_count` | Total persisted clicks |
| `pending_count` | Clicks waiting to be flushed to database |

#### Error Responses

| Status | Code | Error Message |
|--------|------|---------------|
| 400 | `INVALID_SHORT_CODE` | `short code is required` |
| 404 | `NOT_FOUND` | `URL not found` |

---

### Health Check

Kubernetes liveness probe.

```
GET /health
```

#### Response (200 OK)

```json
{
  "status": "healthy",
  "timestamp": "2024-01-02T10:30:45Z"
}
```

---

### Readiness Check

Kubernetes readiness probe with dependency checks.

```
GET /ready
```

#### Response (200 OK)

```json
{
  "status": "ready",
  "timestamp": "2024-01-02T10:30:45Z",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

#### Response (503 Service Unavailable)

```json
{
  "status": "not ready",
  "timestamp": "2024-01-02T10:30:45Z",
  "checks": {
    "database": "ok",
    "redis": "fail"
  }
}
```

---

### Prometheus Metrics

Exposes Prometheus metrics for monitoring.

```
GET /metrics
```

Returns Prometheus text format metrics.

---

## Duration Format

The `expires_in` field accepts Go duration format:

| Format | Example | Description |
|--------|---------|-------------|
| `s` | `30s` | 30 seconds |
| `m` | `10m` | 10 minutes |
| `h` | `24h` | 24 hours |
| `d` | `7d` | 7 days (custom, converted to hours) |

Combinations are also supported: `1h30m`, `2h45m30s`

---

## URL Validation Rules

The following URLs will be rejected:

1. **Empty URLs** - URL field is required
2. **Invalid format** - Must be a valid URL with scheme
3. **Dangerous schemes** - `javascript:`, `data:`, `vbscript:`, `file:`
4. **Private IPs** (by default) - `10.x.x.x`, `192.168.x.x`, `127.0.0.1`, etc.
5. **Blocked hosts** - Configured via `SECURITY_BLOCKED_HOSTS`
6. **Too long** - Maximum 2048 characters (configurable)

---

## SDK Examples

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type ShortenRequest struct {
    URL       string `json:"url"`
    ExpiresIn string `json:"expires_in,omitempty"`
}

type ShortenResponse struct {
    ShortURL    string `json:"short_url"`
    ShortCode   string `json:"short_code"`
    OriginalURL string `json:"original_url"`
    CreatedAt   string `json:"created_at"`
    ExpiresAt   string `json:"expires_at,omitempty"`
}

func main() {
    req := ShortenRequest{
        URL:       "https://example.com/long-url",
        ExpiresIn: "24h",
    }

    body, _ := json.Marshal(req)
    resp, err := http.Post(
        "http://localhost:8080/api/v1/shorten",
        "application/json",
        bytes.NewReader(body),
    )
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    var result ShortenResponse
    json.NewDecoder(resp.Body).Decode(&result)
    println(result.ShortURL)
}
```

### Python

```python
import requests

# Create short URL
response = requests.post(
    "http://localhost:8080/api/v1/shorten",
    json={
        "url": "https://example.com/long-url",
        "expires_in": "24h"
    }
)
data = response.json()
print(data["short_url"])

# Get analytics
response = requests.get(f"http://localhost:8080/api/v1/analytics/{data['short_code']}")
stats = response.json()
print(f"Clicks: {stats['click_count']}")
```

### JavaScript/TypeScript

```typescript
// Create short URL
const response = await fetch('http://localhost:8080/api/v1/shorten', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    url: 'https://example.com/long-url',
    expires_in: '24h'
  })
});

const data = await response.json();
console.log(data.short_url);

// Get analytics
const stats = await fetch(`http://localhost:8080/api/v1/analytics/${data.short_code}`)
  .then(r => r.json());
console.log(`Clicks: ${stats.click_count}`);
```

### cURL

```bash
# Create short URL
curl -X POST http://localhost:8080/api/v1/shorten \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "expires_in": "24h"}'

# Get URL info
curl http://localhost:8080/api/v1/urls/abc1234

# Get analytics
curl http://localhost:8080/api/v1/analytics/abc1234

# Delete URL
curl -X DELETE http://localhost:8080/api/v1/urls/abc1234

# Redirect (follow redirects)
curl -L http://localhost:8080/abc1234
```
