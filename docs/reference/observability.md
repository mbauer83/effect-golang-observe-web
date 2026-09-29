# Observability reference

`observability` is the standard arrangement for watching a service built on
effect-golang-web. One set of settings names what to watch, and each part is
off while it is empty:

| Setting | Watches |
| --- | --- |
| `TracesTo` | every request a span named for its route, exported over OTLP/HTTP to that endpoint |
| `MetricsAt` | runtime metrics served at that path for a Prometheus scrape, prefixed with `Namespace` |
| `InspectAt` | the [runtime inspector](inspect.md) mounted at that path |

`Service` names the program on every span and series. With nothing named, no
observer is attached, and a runtime with no observer builds no events at all.

## Four steps, in the order a program has them

```go
watch, err := observability.New(settings, web.DeclarationsOf(routes...))
runtime, err := watch.Runtime()
surface, err := observability.Surface(watch, reject, routes...)
serving := observability.Deliver[effect.Unit, fault.Fault](watch, scope).AndThen(serve(surface))
```

1. `New` is made before the runtime, because the observers are the runtime's.
   The routes' declarations name the series and spans, so a route added to the
   surface is measured under its own name without being listed twice.
   Declaring routes runs none of their handlers, so they can be declared from
   a service not yet built.
2. `Runtime` builds the runtime with the observers attached, and gives the
   inspector the runtime's live work.
3. `Surface` assembles the program's routes with the scrape and the
   inspector, and traces every request under its route's name. `reject` is how
   a request the codecs refused is responded to, as `web.NewRoutesWithRejection`
   takes it.
4. `Deliver` sends traces for as long as the program's scope lives, and what
   is left when it closes.

## Settings from the environment

`Config` reads `Settings` from the variables OpenTelemetry defines,
`OTEL_SERVICE_NAME` and `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, with
`METRICS_AT`, `METRICS_NAMESPACE` and `INSPECT_AT` beside them:

```go
config.Setting(observability.Config, func(s *Settings, watch observability.Settings) { s.Watch = watch })
```

A program that already reads its own variables for these fills `Settings`
itself.
