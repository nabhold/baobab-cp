# Gate 14 resolution security evidence

| Attack | Control | Executable evidence |
|---|---|---|
| Tenant spoofing | Requested tenant must equal the tenant in the verified workload principal. | `TestResolverHandlerRejectsTenantSpoofing` |
| Native ID injection | Strict request decoding rejects native IDs and all caller-supplied registry state. | `TestResolverHandlerRejectsNativeIDInjection` |
| Cross-context mapping | Mapping scope must be compatible with the trusted Context. | `TestMappingResolverEnforcesScopeAndTemporalWindow` |
| Authorization bypass | The handler independently requires workload identity and `context:resolve`, in addition to router middleware. | `TestResolverHandlerRejectsAuthorizationBypass` |
| Cache isolation | Resolution has no correctness dependency on a cache; any future cache must use trusted Context and registry revision keys. | `TestResolutionHasNoCorrectnessDependencyOnCache` |
| Retired instance routing | Exact selected instance lifecycle is revalidated on every route. | `TestEngineStateChangeIsRevalidated` |

The public resolver accepts only a tenant assertion. Context, Mappings, CapabilityBindings, EngineInstances, native identifiers, and policy state must come from authenticated identity and authoritative Control Plane repositories.
