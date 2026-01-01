# FastGoLink - High-Performance URL Shortener

FastGoLink is a **production-grade, high-performance URL shortening service** built in **Go (Golang)**. It is designed to handle **very high read traffic**, deliver **millisecond-level redirects**, and scale horizontally with minimal operational overhead.

This project focuses on **real-world backend engineering concerns** rather than CRUD basics.

---

## Features

- **Ultra-fast redirects** - Redis-first caching delivers sub-5ms response times
- **Secure URL validation** - Blocks dangerous schemes, private IPs, and configurable blocklists
- **Click analytics** - Non-blocking analytics with async batch persistence
- **Rate limiting** - IP-based and API key-based rate limiting with sliding window
- **Health monitoring** - Kubernetes-ready liveness and readiness probes
- **Prometheus metrics** - Full observability with request metrics and latency histograms
- **API Documentation** - Interactive Scalar, ReDoc, and Swagger UI documentation

---

## Quick Start

### Prerequisites

- Go 1.24+
- PostgreSQL 14+ (optional, for persistence)
- Redis 7+ (optional, for caching)
- Docker & Docker Compose (optional)

### Running with Docker

```bash
# Start all services (PostgreSQL, Redis, API)
docker-compose up -d

# View logs
docker-compose logs -f api
```

### Running Locally

```bash
# Install dependencies
go mod download

# Run the server (in-memory mode)
go run cmd/api/main.go

# Or with database and cache
export DB_HOST=localhost DB_PASSWORD=your_password REDIS_HOST=localhost
go run cmd/api/main.go
```

The server starts on `http://localhost:8080` by default.

---

## API Documentation

FastGoLink provides interactive API documentation through multiple interfaces:

| Documentation | URL | Description |
|--------------|-----|-------------|
| **Scalar** (Recommended) | [`/docs`](http://localhost:8080/docs) | Modern, feature-rich API documentation |
| **ReDoc** | [`/docs/redoc`](http://localhost:8080/docs/redoc) | Clean, three-panel documentation |
| **Swagger UI** | [`/docs/swagger`](http://localhost:8080/docs/swagger) | Classic Swagger interface with "Try it out" |
| **OpenAPI Spec** | [`/docs/openapi.yaml`](http://localhost:8080/docs/openapi.yaml) | Raw OpenAPI 3.0 specification |

### API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/shorten` | Create a new short URL |
| `GET` | `/api/v1/urls/:code` | Get URL information and stats |
| `DELETE` | `/api/v1/urls/:code` | Delete a short URL |
| `GET` | `/:code` | Redirect to original URL |
| `GET` | `/api/v1/analytics/:code` | Get click statistics |
| `GET` | `/health` | Liveness probe |
| `GET` | `/ready` | Readiness probe with dependency checks |
| `GET` | `/metrics` | Prometheus metrics |

### Quick API Examples

#### Create Short URL

```bash
curl -X POST http://localhost:8080/api/v1/shorten \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/very/long/path", "expires_in": "24h"}'
```

Response:
```json
{
  "short_url": "http://localhost:8080/abc1234",
  "short_code": "abc1234",
  "original_url": "https://example.com/very/long/path",
  "created_at": "2024-01-02T10:30:45Z",
  "expires_at": "2024-01-03T10:30:45Z"
}
```

#### Redirect

```bash
curl -L http://localhost:8080/abc1234
# Redirects to https://example.com/very/long/path
```

#### Get Analytics

```bash
curl http://localhost:8080/api/v1/analytics/abc1234
```

Response:
```json
{
  "short_code": "abc1234",
  "click_count": 1523,
  "pending_count": 12
}
```

---

## Architecture

```
Client
  |
  v
Load Balancer
  |
  v
Go API Servers (stateless, multiple instances)
  |
  +---> Redis Cache (hot cache, < 5ms lookups)
  |     - Cache-first reads
  |
  +---> PostgreSQL Database (durable storage)
  |     - Persistent URL mappings
  |
  +---> Analytics Pipeline
        - Async batch processing
```

### Request Flows

**Create Short URL:**
1. Client submits URL via POST /api/v1/shorten
2. Backend validates and sanitizes the URL
3. Generates unique short code (Base62, cryptographically secure)
4. Stores mapping in database
5. Caches in Redis (write-through)
6. Returns shortened URL

**Redirect (Critical Path):**
1. Incoming request hits GET /:code
2. Check Redis cache
3. Cache hit → immediate redirect (1-5ms)
4. Cache miss → DB lookup → cache → redirect (10-50ms)
5. Analytics recorded asynchronously (non-blocking)

---

## Configuration

Configuration is loaded from environment variables with sensible defaults:

### Application

| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | `development` | Environment mode |
| `LOG_LEVEL` | `info` | Log level (debug/info/warn/error) |

### Server

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_HOST` | `0.0.0.0` | Bind address |
| `SERVER_PORT` | `8080` | Port number |
| `SERVER_READ_TIMEOUT` | `5s` | Request read timeout |
| `SERVER_WRITE_TIMEOUT` | `10s` | Response write timeout |
| `SERVER_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |

### Database (PostgreSQL)

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `localhost` | PostgreSQL hostname |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `gourl` | Database user |
| `DB_PASSWORD` | - | Database password |
| `DB_NAME` | `gourl` | Database name |
| `DB_SSLMODE` | `disable` | SSL mode |
| `DB_MAX_OPEN_CONNS` | `25` | Max open connections |
| `DB_MAX_IDLE_CONNS` | `5` | Max idle connections |
| `DB_CONN_MAX_LIFETIME` | `5m` | Connection max lifetime |

### Redis

| Variable | Default | Description |
|----------|---------|-------------|
| `REDIS_HOST` | `localhost` | Redis hostname |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PASSWORD` | - | Redis password |
| `REDIS_DB` | `0` | Redis database index |
| `REDIS_POOL_SIZE` | `10` | Connection pool size |
| `REDIS_KEY_PREFIX` | `url:` | Cache key prefix |
| `REDIS_CACHE_TTL` | `24h` | Cache time-to-live |

### URL Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `URL_BASE_URL` | `http://localhost:8080` | Base URL for short links |
| `URL_SHORT_CODE_LEN` | `7` | Short code length |
| `URL_IDGEN_STRATEGY` | `random` | ID generation strategy |
| `URL_IDGEN_MAX_RETRIES` | `3` | Collision retry attempts |

### Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `RATE_LIMIT_ENABLED` | `true` | Enable rate limiting |
| `RATE_LIMIT_REQUESTS` | `100` | Requests per window |
| `RATE_LIMIT_WINDOW` | `1m` | Rate limit window |
| `RATE_LIMIT_TRUST_PROXY` | `false` | Trust X-Forwarded-For |
| `RATE_LIMIT_API_KEY_HEADER` | `X-API-Key` | API key header name |

### Security

| Variable | Default | Description |
|----------|---------|-------------|
| `SECURITY_MAX_URL_LENGTH` | `2048` | Max URL length |
| `SECURITY_ALLOW_PRIVATE_IPS` | `false` | Allow private IP targets |
| `SECURITY_BLOCKED_HOSTS` | - | CSV of blocked hosts |

---

## Project Structure

```
FastGoLink/
├── cmd/api/main.go                    # Application entry point
├── internal/                           # Private application code
│   ├── analytics/                      # Click tracking & async processing
│   ├── cache/                          # Redis caching layer
│   ├── config/                         # Configuration management
│   ├── database/                       # PostgreSQL layer & sharding
│   ├── handlers/                       # HTTP request handlers
│   ├── idgen/                          # ID generation strategies
│   ├── metrics/                        # Prometheus metrics
│   ├── middleware/                     # HTTP middleware
│   ├── models/                         # Domain models
│   ├── ratelimit/                      # Rate limiting logic
│   ├── repository/                     # Data persistence layer
│   ├── security/                       # URL sanitization & validation
│   ├── server/                         # HTTP server setup
│   └── services/                       # Business logic layer
├── pkg/                                # Public packages (logger)
├── tests/                              # Comprehensive test suite
│   ├── unit/                           # Unit tests
│   ├── integration/                    # Integration tests
│   ├── e2e/                            # End-to-end tests
│   └── benchmark/                      # Performance benchmarks
├── docs/                               # API documentation
│   └── openapi.yaml                    # OpenAPI 3.0 specification
├── migrations/                         # Database migrations
└── configs/                            # Configuration templates
```

---

## Performance

| Metric | Target | Achieved |
|--------|--------|----------|
| Redirect latency (cache hit) | < 10ms | 1-5ms |
| Redirect throughput | 50k-100k req/sec | Horizontal scalable |
| URL creation latency | < 100ms | 20-50ms |
| Cache TTL | 24 hours | Configurable |

---

## Testing

```bash
# Run all tests
make test

# Run unit tests only
make test-unit

# Run integration tests (requires Docker)
make test-integration

# Run end-to-end tests
make test-e2e

# Run benchmarks
make benchmark

# Generate coverage report
make coverage
```

---

## Development

### Prerequisites

- Go 1.24+
- Docker & Docker Compose
- Make

### Local Development

```bash
# Start dependencies
docker-compose up -d postgres redis

# Run with hot reload (using air)
go install github.com/cosmtrek/air@latest
air

# Or run directly
go run cmd/api/main.go
```

### Code Quality

```bash
# Format code
gofmt -w .

# Lint code
golangci-lint run

# Run all checks
make check
```

---

## Deployment

### Docker

```bash
# Build production image
docker build -t fastgolink:latest .

# Run production container
docker run -p 8080:8080 \
  -e DB_HOST=your-db-host \
  -e DB_PASSWORD=your-db-password \
  -e REDIS_HOST=your-redis-host \
  fastgolink:latest
```

### Docker Compose (Production)

```bash
docker-compose -f docker-compose.prod.yml up -d
```

### Kubernetes

The service is designed for Kubernetes deployment:
- Stateless API servers for horizontal scaling
- Health endpoints for liveness/readiness probes
- Prometheus metrics for observability
- Graceful shutdown handling

---

## Security Features

- **URL Validation**: Blocks dangerous schemes (javascript:, data:, vbscript:, file:)
- **Private IP Blocking**: Prevents SSRF attacks by blocking private IPs
- **Configurable Blocklist**: Block specific hosts/domains
- **Rate Limiting**: IP and API key-based rate limiting
- **Input Sanitization**: URL normalization and validation

---

## Monitoring

### Prometheus Metrics

Available at `/metrics`:
- `http_requests_total` - Request counters by method, path, status
- `http_request_duration_seconds` - Request latency histogram
- `cache_hits_total` / `cache_misses_total` - Cache performance
- `db_query_duration_seconds` - Database latency
- `rate_limit_hits_total` - Rate limit triggers

### Health Checks

- `GET /health` - Liveness probe (is the service running?)
- `GET /ready` - Readiness probe (can the service handle traffic?)

---

## License

MIT License - See [LICENSE](LICENSE) for details.

---

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting a pull request.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request
