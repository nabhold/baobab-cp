# Gate 11 resolution performance baseline

Measured on GitHub Actions run 34244781980 at commit `6ae8d129b33557ff7678d2831d42c9ffd21610af`.

## Workload

The benchmark resolves one canonical mapping from 100 candidates. Every candidate is validated for lifecycle, temporal validity, and tenant scope. Exactly one candidate is scope-compatible. The benchmark uses the production resolver code and reports allocations.

## Environment

- OS/architecture: Linux amd64
- CPU: AMD EPYC 7763 64-Core Processor
- Samples: 5
- Go benchmark parallelism suffix: 4

## Results

| Sample | ns/op | Approx. operations/second | B/op | allocs/op |
|---:|---:|---:|---:|---:|
| 1 | 33,299 | 30,031 | 28,888 | 102 |
| 2 | 32,775 | 30,511 | 28,888 | 102 |
| 3 | 32,622 | 30,654 | 28,888 | 102 |
| 4 | 32,832 | 30,458 | 28,888 | 102 |
| 5 | 32,648 | 30,630 | 28,888 | 102 |

Median latency was 32,775 ns/op (32.775 µs), corresponding to approximately 30,511 operations/second for this single-threaded benchmark workload.

## Recommendation

Use 100 µs as a provisional **internal engineering budget** for pure in-memory resolution of 100 candidates on comparable hardware. This is about three times the measured median and is intended to detect regressions, not to promise customer-visible latency.

Do not publish an external API SLO from this microbenchmark. End-to-end latency also includes authentication, PostgreSQL/cache access, network queues, serialization, and policy evaluation. Establish the service SLO only after representative load tests in the production-like environment provide p50, p95, and p99 distributions.

Re-run the five-sample benchmark after resolver algorithm, Go toolchain, or runner-class changes. Investigate either a median above 100 µs or a material allocation increase from the 28,888 B/op and 102 allocs/op baseline.
