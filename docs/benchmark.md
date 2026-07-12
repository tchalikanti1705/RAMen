# Benchmarking RAMen

This guide shows how to benchmark a local RAMen instance using `redis-benchmark`.

## Prerequisites

Start RAMen using Docker Compose:

```bash
docker compose up
```

This exposes:

- RESP server: `localhost:6379`
- Dashboard: `http://localhost:8080`

## Running benchmarks

### Linux/macOS

```bash
redis-benchmark -h localhost -p 6379 \
  -t ping,set,get,incr,lpush,rpush,lpop,sadd,hset,zadd
```

### Windows (PowerShell)

```powershell
docker run --rm redis redis-benchmark -h host.docker.internal -p 6379 -t ping,set,get,incr,lpush,rpush,lpop,sadd,hset,zadd
```

## Notes

- `redis-benchmark` may print:

```
WARNING: Could not fetch server CONFIG
```

This warning is expected because redis-benchmark tries to call Redis's CONFIG command to gather server information before running benchmarks. If RAMen doesn't implement CONFIG, it prints a warning but continues benchmarking normally. It does not prevent the benchmarks from running.

- The benchmark categories listed above were verified against the Docker Compose setup.
- No unsupported benchmark categories were identified during testing.
