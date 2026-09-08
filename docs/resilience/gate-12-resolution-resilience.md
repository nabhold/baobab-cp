# Gate 12 resolution resilience

The resolver fails closed when authoritative state cannot produce one unambiguous route.

| Failure mode | Safe behavior | Evidence |
|---|---|---|
| PostgreSQL transient failure | Resolution service returns a wrapped error; it never substitutes request-supplied state after repository access fails. | `TestResolutionServiceRepositoryFailureFailsClosed` |
| Cache failure | The authoritative pipeline has no correctness dependency on a cache. | `TestResolutionHasNoCorrectnessDependencyOnCache` |
| Engine-instance state change | Lifecycle and health are revalidated for every resolution. | `TestEngineStateChangeIsRevalidated` |
| Duplicate request | Identical input produces an identical decision without mutation. | `TestDuplicateResolutionRequestsAreDeterministic` |
| Concurrent binding change | Equal-ranked active bindings fail as ambiguous. | `TestConcurrentBindingChangeFailsClosed` |

Caching may be added only as an optimization. Any future cache hit must be scoped by trusted Context, tied to an authoritative registry revision, and revalidate the selected EngineInstance before use.
