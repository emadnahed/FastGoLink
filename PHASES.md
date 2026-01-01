# GoURL Development Phases

Production-grade URL Shortener built with **strict TDD methodology**.

---

## Phase 0: Setup (COMPLETE)

**Goal**: Production-ready project foundation

### Deliverables
- [x] Git repository with `.gitignore`
- [x] Go module (`go.mod`)
- [x] Folder structure (cmd, internal, pkg, tests, configs)
- [x] Linting configuration (`.golangci.yml`)
- [x] Editor configuration (`.editorconfig`)
- [x] Test frameworks (unit, integration, E2E)
- [x] Environment configuration (`.env.example`)
- [x] Makefile with common commands

### No Business Logic
Phase 0 contains zero business logic - only infrastructure.

---

## Phase 1: Core HTTP Server & Health Checks (COMPLETE)

**Goal**: Deployable HTTP server with health endpoints

### Features
- [x] HTTP server with graceful shutdown
- [x] Health check endpoint (`GET /health`)
- [x] Readiness probe endpoint (`GET /ready`)
- [x] Structured JSON logging
- [x] Configuration loading from environment

### TDD Approach
1. Write failing tests for health endpoints
2. Implement minimal HTTP server
3. Write failing tests for graceful shutdown
4. Implement shutdown handling
5. Refactor and verify all tests pass

### Tests Required
- **Unit**: Config loading, response formatting
- **Integration**: Server startup/shutdown
- **E2E**: HTTP requests to health endpoints

---

## Phase 2: Database Layer (PostgreSQL) (COMPLETE)

**Goal**: Persistent storage with migrations

### Features
- [x] Database connection pool
- [x] Migration system (up/down)
- [x] URL model and repository interface
- [x] Connection health checks

### Schema
```sql
CREATE TABLE urls (
    id BIGSERIAL PRIMARY KEY,
    short_code VARCHAR(10) UNIQUE NOT NULL,
    original_url TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    click_count BIGINT DEFAULT 0
);

CREATE INDEX idx_urls_short_code ON urls(short_code);
CREATE INDEX idx_urls_expires_at ON urls(expires_at) WHERE expires_at IS NOT NULL;
```

### TDD Approach
1. Write failing tests for repository interface
2. Implement PostgreSQL repository
3. Write failing tests for migrations
4. Implement migration system
5. Integration tests with real database

### Tests Required
- **Unit**: SQL query building, model validation
- **Integration**: Repository CRUD operations
- **E2E**: Full database connectivity

---

## Phase 3: Redis Cache Layer (COMPLETE)

**Goal**: High-performance caching for redirects

### Features
- [x] Redis connection pool
- [x] Cache interface with TTL support
- [x] Write-through caching strategy
- [x] Cache invalidation
- [x] Fallback to database on cache miss

### TDD Approach
1. Write failing tests for cache interface
2. Implement Redis cache
3. Write failing tests for cache miss scenarios
4. Implement fallback logic
5. Integration tests with Redis

### Tests Required
- **Unit**: Cache key generation, TTL handling
- **Integration**: Redis operations, failover
- **E2E**: Cache hit/miss flows

---

## Phase 4: ID Generation (COMPLETE)

**Goal**: Unique, URL-safe short codes

### Features
- [x] Base62 encoding/decoding
- [x] Snowflake-style ID generation
- [x] Collision detection and retry
- [x] Configurable code length

### Strategies
- **Base62**: Compact alphanumeric codes (a-z, A-Z, 0-9)
- **Snowflake**: Time + node-based uniqueness for distributed systems

### TDD Approach
1. Write failing tests for Base62 encoding
2. Implement Base62 encoder
3. Write failing tests for uniqueness
4. Implement collision handling
5. Unit tests for edge cases

### Tests Required
- **Unit**: Encoding/decoding, collision handling
- **Integration**: ID generation at scale
- **E2E**: Unique code generation flow

---

## Phase 5: URL Shortening API (COMPLETE)

**Goal**: Create short URLs via API

### Endpoints
- `POST /api/v1/shorten` - Create short URL
- `GET /api/v1/urls/:code` - Get URL info (admin)
- `DELETE /api/v1/urls/:code` - Delete URL (admin)

### Request/Response
```json
// POST /api/v1/shorten
{
  "url": "https://example.com/very/long/path",
  "expires_in": "24h"  // optional
}

// Response
{
  "short_url": "http://localhost:8080/abc1234",
  "short_code": "abc1234",
  "original_url": "https://example.com/very/long/path",
  "expires_at": "2024-01-02T00:00:00Z",
  "created_at": "2024-01-01T00:00:00Z"
}
```

### TDD Approach
1. Write failing tests for URL validation
2. Implement URL validator
3. Write failing tests for shorten endpoint
4. Implement handler → service → repository flow
5. E2E tests for complete API flow

### Tests Required
- **Unit**: URL validation, request parsing
- **Integration**: Service layer with mocked deps
- **E2E**: Full HTTP → DB → cache → response

---

## Phase 6: URL Redirect (Critical Path) (COMPLETE)

**Goal**: Ultra-fast redirects via short codes

### Endpoints
- `GET /:code` - Redirect to original URL

### Flow
1. Check Redis cache
2. Cache hit → 301/302 redirect (< 5ms)
3. Cache miss → Query DB → Update cache → Redirect
4. Not found → 404

### TDD Approach
1. Write failing tests for redirect handler
2. Implement cache-first lookup
3. Write failing tests for cache miss
4. Implement DB fallback
5. Performance tests for latency

### Tests Required
- **Unit**: Redirect logic, status codes
- **Integration**: Cache + DB coordination
- **E2E**: Full redirect flow with timing

---

## Phase 7: Rate Limiting & Security (COMPLETE)

**Goal**: Abuse protection and security hardening

### Features
- [x] IP-based rate limiting (sliding window algorithm)
- [x] API key rate limiting (optional via X-API-Key header)
- [x] Request validation middleware
- [x] SQL injection prevention (parameterized queries - already in place)
- [x] URL sanitization (block dangerous schemes, private IPs)
- [x] Request ID tracking (X-Request-ID header)

### Implementation Details
- **Middleware Chain**: RequestID → ClientIP → RateLimit → Handler
- **Rate Limiter**: In-memory sliding window with configurable requests/window
- **URL Sanitizer**: Blocks javascript:/data:/vbscript:/file: schemes, localhost, private IPs
- **Configuration**: Via environment variables (RATE_LIMIT_*, SECURITY_*)

### Tests Required
- **Unit**: Rate limit algorithm, URL sanitization
- **Integration**: Middleware chain
- **E2E**: Rate limit enforcement, malicious URL blocking

---

## Phase 8: Click Analytics (Async) (COMPLETE)

**Goal**: Non-blocking analytics tracking

### Features
- [x] Click count increment
- [x] Async processing (goroutine/channel)
- [x] Batch updates to reduce DB load
- [x] Basic analytics endpoint (`GET /api/v1/analytics/:code`)

### Implementation Details
- **ClickCounter**: Non-blocking channel-based counter with configurable flush interval and batch size
- **Batch Updates**: Efficient single SQL UPDATE with CASE statement for multiple URLs
- **Repository Integration**: `BatchIncrementClickCounts` added to URLRepository interface
- **Analytics Endpoint**: Returns current click count plus pending (unflushed) clicks
- **Configuration**: Default 10s flush interval, 100 batch size, 10000 channel buffer

### Tests Required
- **Unit**: Counter logic, batch accumulation
- **Integration**: Async processing
- **E2E**: Analytics accuracy over time

---

## Phase 9: Docker & Deployment (COMPLETE)

**Goal**: Production-ready containerization

### Deliverables
- [x] Multi-stage Dockerfile
- [x] docker-compose.yml (dev environment)
- [x] docker-compose.prod.yml
- [x] Health check scripts (via Docker HEALTHCHECK)
- [x] Environment-specific configs

### TDD Approach
1. Write container health tests
2. Build and test containers
3. Write deployment verification tests
4. Implement production configs

---

## Phase 10: Observability

**Goal**: Production monitoring and debugging

### Features
- [ ] Prometheus metrics endpoint
- [ ] Request latency histograms
- [ ] Cache hit/miss ratios
- [ ] Error rate tracking
- [ ] Structured logging with correlation IDs

### Metrics
- `http_requests_total`
- `http_request_duration_seconds`
- `cache_hits_total` / `cache_misses_total`
- `db_query_duration_seconds`
- `active_connections`

---

## Git Workflow

### Branch Strategy
```
main (protected)
└── feature/phase-X-description
    └── PR → main
```

### Commit Convention
- `feat:` New feature
- `test:` Test additions/changes
- `fix:` Bug fixes
- `refactor:` Code restructuring
- `chore:` Build/config changes
- `docs:` Documentation

### Example Commits
```
test: add failing tests for health endpoint
feat: implement health check handler
test: add integration tests for server shutdown
refactor: extract response helper functions
```

---

## Definition of Done

A phase is complete when:

1. All planned features are implemented
2. Unit tests pass with >80% coverage
3. Integration tests pass
4. E2E tests validate real flows
5. Linting passes (`make lint`)
6. Code reviewed and merged to main
7. Documentation updated

---

## Quick Reference

| Phase | Focus Area | Key Deliverable |
|-------|------------|-----------------|
| 0 | Setup | Project infrastructure |
| 1 | HTTP Server | Health endpoints |
| 2 | Database | PostgreSQL + migrations |
| 3 | Cache | Redis integration |
| 4 | ID Gen | Short code generation |
| 5 | Shorten API | Create short URLs |
| 6 | Redirect | Fast redirects |
| 7 | Security | Rate limiting |
| 8 | Analytics | Click tracking |
| 9 | Docker | Containerization |
| 10 | Observability | Metrics + logging |
