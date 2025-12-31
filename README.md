# 🚀 GoURL — High-Performance URL Shortener (Go Backend)

GoURL is a **production-grade, high-performance URL shortening service** built in **Go (Golang)**.
It is designed to handle **very high read traffic**, deliver **millisecond-level redirects**, and scale horizontally with minimal operational overhead.

This project focuses on **real-world backend engineering concerns** rather than CRUD basics.

---

## 🧠 Why This Project Matters

URL shorteners are deceptively simple systems that stress:
- **Read-heavy traffic**
- **Low-latency redirects**
- **Caching correctness**
- **ID generation at scale**
- **Horizontal scalability**

GoURL demonstrates how modern backend systems are actually designed in industry.

---

## 🎯 Core Features

- ⚡ Ultra-fast redirects (Redis-first)
- 🔗 Short URL generation using Base62 / Snowflake-style IDs
- 🧵 High concurrency using Go goroutines
- 🗄 Persistent storage with PostgreSQL / DynamoDB
- 🚀 Horizontal scaling with stateless servers
- 📊 Click tracking (optional async)
- 🔐 Rate limiting & abuse protection
- 🧠 Cache warming and TTL-based eviction

---

## 🏗 High-Level Architecture

Client  
→ Load Balancer  
→ Go API Servers (stateless)  
→ Redis (hot cache)  
→ Primary Database (durable storage)

Redirect traffic **never blocks on database reads** unless cache misses occur.

---

## 🔄 Request Flows

### 1️⃣ Create Short URL
1. Client submits a long URL
2. Backend validates and normalizes the URL
3. Generates a unique short code
4. Stores mapping in the database
5. Writes mapping to Redis
6. Returns shortened URL

### 2️⃣ Redirect (Critical Path)
1. Incoming request hits short URL
2. Redis lookup
3. Cache hit → immediate redirect (1–5 ms)
4. Cache miss → DB → Redis → redirect

---

## 🧮 ID Generation Strategy

Supported strategies:
- **Base62 encoding** (compact, URL-safe)
- **Snowflake-style IDs** (time + node-based uniqueness)

Collision handling is deterministic and safe.

---

## 🗄 Data Model (Simplified)

```
urls
- id (bigint / uuid)
- short_code (unique)
- original_url
- created_at
- expires_at (optional)
- click_count
```

---

## ⚙️ Performance Characteristics

- **Latency (cache hit):** < 10 ms
- **Throughput:** 50k–100k req/sec per node
- **Scaling:** Horizontal (stateless API)
- **Failure Mode:** Graceful degradation on cache loss

---

## 🔐 Security & Reliability

- Input validation & URL sanitization
- Rate limiting per IP / API key
- Idempotent URL creation
- Graceful shutdowns
- Structured logging
- Health checks & readiness probes

---

## 🧰 Tech Stack

- **Language:** Go
- **HTTP:** net/http / Gin / Fiber
- **Cache:** Redis
- **Database:** PostgreSQL or DynamoDB
- **Containerization:** Docker
- **Observability:** Prometheus + Grafana

---

## 📦 Project Structure

```
cmd/
  api/
internal/
  handlers/
  services/
  repository/
  cache/
  idgen/
pkg/
configs/
```

Clean separation between **transport**, **business logic**, and **infrastructure**.

---

## 🚀 Running Locally

```bash
docker-compose up
go run cmd/api/main.go
```

---

## 🌍 Production Readiness Notes

- Stateless services for easy scaling
- Redis as a performance boundary
- Database protected from redirect storms
- Ready for Kubernetes / ECS / Nomad

---

## 📈 Future Enhancements

- Geo-based analytics
- Custom aliases
- Link expiration
- Admin dashboard
- Bulk shortening API
- Kafka-based async analytics

---

## 🧠 What This Project Demonstrates

- Backend system design
- Performance-first thinking
- Caching strategies
- Concurrency handling in Go
- Real-world scalability tradeoffs

---

## 📜 License

MIT License
