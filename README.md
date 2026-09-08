# effect-golang-observe-web

Looking at a running [effect-golang](https://github.com/mbauer83/effect-golang)
program: a page, a described snapshot behind it, and the one line that makes a
web program's traffic visible.

[effect-golang-observe](https://github.com/mbauer83/effect-golang-observe) reads
a runtime's events. This puts the reading where a person can see it, and adds
the two things only a web module can:

- **it knows what is being served.** A `web.Routes` carries the same
  declarations that dispatch a request and project into an OpenAPI document, so
  the inspector shows the surface beside the traffic through it.
- **it can name a request.** A runtime brackets what a program tells it to
  bracket, so a handler that opens no span of its own contributes nothing to a
  trace. One setting on the assembled surface makes every request a span named
  for its route — the *pattern*, never the path that was asked for.

The inspector is itself an effect-golang-web program, mounted inside the one it
inspects. That is not a flourish: a tool for inspecting this stack that was not
built on it would be a tool nobody had tried the stack with.

## Status

| Area | State |
|---|---|
| [The inspector: page, snapshot, contract](docs/reference/inspect.md) | usable |
| [Naming requests: `Observing`, `Names`](docs/reference/inspect.md) | usable; one setting on the surface, nothing at the call sites |
| [Memory and compute: gauges, charts, cost per route and stage](docs/reference/inspect.md) | usable; process-wide, because Go reports no per-goroutine allocation or CPU |
| [The page: timeline, hot paths, fibers, spans, surface](docs/reference/inspect.md) | usable; charts by vendored uPlot, served from the inspector |
| Push protocol to an out-of-process tool | absent, and [deliberately](docs/reference/inspect.md) |

## The shortest useful thing

```go
watched := inspect.Watched{
    Running:   trace.Watch(),
    Fibers:    trace.WatchFibers(),
    Window:    window,
    Collected: metrics.Collect(metrics.Naming(inspect.Names(declarations)...)),
    Owned:     runtime.LiveWork,
}

// The routes, written exactly as they would be without any of this.
mine := []web.Route[effect.Unit, Refusal]{
    web.Handle(ListNotes, list),
    web.Handle(AddNote, add),
}
described, _ := web.NewRoutes(mine...)
watched.Surface = described.Declarations

inspecting, _ := inspect.Routes[effect.Unit, Refusal](watched, inspect.DefaultAt)
surface, _ := web.NewRoutes(append(mine, inspecting...)...)

// One setting. Not applying it is how a program turns observation off.
surface = surface.Wrapping(inspect.Observing[effect.Unit, Refusal](watched.Costs))
```

One surface, so one matcher dispatches everything: the inspector is not a
second server on a second port to be found, configured and secured separately.
Then open `/inspect`.

## Layout

```text
inspect/                    Watched, Snapshot, Surface, Routes, Observing
inspect/page.html           the page, one checked-in document
inspect/assets/             the vendored chart library, and its licence
examples/inspected/         a small web program with the inspector mounted
examples/cmd/inspectdemo/   the example as a runnable command
test/unit/                  the wire conversion
test/acceptance/            the inspector inside the program, over a socket
test/architecture/          the claims about this module's shape
docs/                       reference
```

## What it produces

`go run ./examples/cmd/inspectdemo` starts the program, sends it five requests
and reads the inspector's own snapshot back through the client:

```text
  10 span(s) in the window, 4 event(s) outside every span
  GET /notes                   +0s         53µs       success
  POST /notes                  +420µs      14µs       success
  GET /notes/{title}           +1.008ms    11µs       success
  GET /notes/report            +1.519ms    109µs      success
    read                         +1.542ms    33µs       success
    count                        +1.58ms     7µs        success
    digest                       +1.59ms     20µs       success

  what the process spent while each route ran
    GET /notes/report        x10   28.2KiB per run, longest 217µs
    rank                     x10    4.9KiB per run, longest  26µs
    read                     x10    3.8KiB per run, longest  59µs
```

Every request under its route's **pattern**, with an offset and a duration, and
the stages of the one that has them. Two requests to `/notes/First` and
`/notes/Missing` are one name, because a series per title is a series per
request. `read` is the hottest stage by time and `digest` and `rank` by
allocation — which is the distinction the hot-path panel exists for.

## Documentation

- [The inspector](docs/reference/inspect.md) — what it serves, what it shows,
  and what it deliberately is not

## Development

`go.mod` requires the modules below this one by version, so what a consumer
resolves is what this module was built against. Working on several at once is a
workspace's job:

```sh
cd workspace
go work init ./effect-golang ./effect-golang-schema ./effect-golang-web \
             ./effect-golang-observe ./effect-golang-observe-web
```

Releasing is
[RELEASING.md](https://github.com/mbauer83/effect-golang/blob/main/RELEASING.md).

## Scope

A tool is installed in the program it watches, so every dependency it brings is
one that program now has — and a version conflict between an inspector and the
thing it inspects is the worst possible time to have one. So this module
carries no Go dependency outside this project, and the page fetches nothing at
run time.

The one third-party thing it carries is [uPlot](https://github.com/leeoniya/uPlot),
vendored under `inspect/assets` and served by the inspector: fifty kilobytes of
MIT-licensed JavaScript with no dependencies of its own, for axes, a cursor and
a legend. Its licence is checked in beside it and served alongside it, and an
architecture test asserts both.
