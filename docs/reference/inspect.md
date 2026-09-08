# Inspector reference

```go
inspect.Watching(endpoint, handle) web.Route[R, E]         // web.Handle, plus a span
inspect.Accounted(costs, endpoint, handle) web.Route[R, E] // and what it spent
inspect.Names(declarations) []string                       // the metric vocabulary
inspect.NameOf(declaration) string

inspect.Routes[R, E](watched, at) ([]web.Route[R, E], error)
inspect.Surface[R, E](watched, at) (web.Routes[R, E], error)

inspect.Watched{Running, Fibers, Window, Collected, Queued, Owned, Surface}
func (watched Watched) Take(now time.Time) Snapshot
inspect.Read(entity) (Snapshot, error)
inspect.Write(taken) ([]byte, error)
inspect.Page() []byte
inspect.Script() []byte      // the vendored chart library
inspect.Stylesheet() []byte
inspect.Licence() []byte
```

## Naming a request

A runtime brackets what a program tells it to bracket. A handler that opens no
span of its own contributes nothing to a trace — a request appears as a fiber
that ran and nothing more. `Watching` is a drop-in for `web.Handle` and is the
one line that changes that for a whole surface.

**The name is the method and the route's pattern**, `GET /books/{title}`, and
never the path that was asked for. A concrete path is an unbounded value, and
the rule that forbids it as a
[metric label](https://github.com/mbauer83/effect-golang-observe/blob/main/docs/reference/metrics.md)
forbids it as a span name: a series per title is a series per request.

`Names` closes the loop. The vocabulary of a bounded aggregate comes from the
same declarations that dispatch the requests, so a route added to the surface
is measured without anybody remembering it in a second place:

```go
metrics.Collect(metrics.Naming(inspect.Names(surface.Declarations())...))
```

## Mounting

`Routes` gives the list, for assembling with a program's own; `Surface` gives
them assembled, for a program that serves nothing else. One surface is the
point: one matcher dispatches everything, an ambiguity between the inspector
and the program is reported at start-up like any other, and there is no second
port to find, configure and secure.

Three endpoints under the mount point — `/inspect` unless a caller says
otherwise:

| Path | Answers |
|---|---|
| `/inspect` | the page, one embedded HTML document |
| `/inspect/snapshot` | one reading, described by `SnapshotSchema` |
| `/inspect/openapi.json` | the inspector's own contract |
| `/inspect/uplot.js`, `/inspect/uplot.css`, `/inspect/uplot.LICENSE` | the vendored chart library |

`Surface` and `Routes` are **generic in the failure channel**. The inspector's
own handlers cannot fail, so they mount inside any program's routes whatever
that program fails with — and the program's refusals stay the program's, mapped
to statuses by its own boundary. `inspect.Fault` is returned from assembly as a
Go error and is never a failure channel.

## What a reading contains

`Watched` names the parts and every one is optional: an inspector with no
collector shows no measurements rather than refusing to start.

| Part | Question |
|---|---|
| `Fibers` | which fibers are running, nested as forked, with ages |
| `Owned` | what the runtime still holds (`runtime.LiveWork`) |
| `Running` | which spans are open, with ages |
| `Window` | the recent events, folded into a trace |
| `Collected` | bounded counts, durations and delays |
| `Queued` | how many events the queue discarded |
| `Series` | memory and compute over time |
| `Costs` | what the process spent while each name's work ran |
| `Surface` | the routes being served |

**A `Watched` is handed over by value, so it must be complete when `Routes` or
`Surface` is called.** Assigning a field afterwards assigns it to a copy the
inspector has already taken. `Owned` and `Surface` are functions, so what a
reading reports is what they say at that moment — and a surface that includes
the inspector's own routes is expressed by closing over the variable the
assembled routes will land in.

**A reading is nearly consistent, not consistent.** The live spans, the fibers,
the window and the aggregate are read one after another, and a program does not
stop in between. That is what any tool reading a running program gets, and it
is worth saying rather than implying.

## The wire shape

Its own shapes rather than the ones `observe` holds, for two reasons a page
makes obvious. **A span tree is recursive** and a description of it would be
too, so the tree is flattened and its shape carried in a `Depth` — which is
also what a page renders: a list, indented. And **a duration is microseconds as
a whole number**, because a page does arithmetic on it and `3.879µs` is prose.

`StartMicros` is on every span because
[Effect's devtools schema](https://github.com/mbauer83/effect-golang-observe/blob/main/docs/explanation/prior-art.md)
puts a start time on every span rather than a duration alone, and that is what
a waterfall needs: a duration says how long, and only an offset says *when*.
The offsets are relative to the earliest span in the reading, because an
absolute clock reading would make the page's arithmetic depend on whose clock
it was. `AgeMicros` is the other half, for a span that has not ended and
therefore has no duration to give.

`Owned.Counted` says whether the runtime was built to keep its counters — off
by default, since they cost a pair of atomics per fiber and per resource. Two
zeroes and "nobody is counting" look identical otherwise.

`SelfMicros` is a span's duration less its children's. That is what "hot"
means: a route that spends all of itself inside one stage is not where the
time went, the stage is, and ranking by total would blame the route.

`Owned.Counted` says whether the runtime was built to keep its counters — off
by default, since they cost a pair of atomics per fiber and per resource. Two
zeroes and "nobody is counting" look identical otherwise.

`Read` and `Write` are public because the inspector's own client should be a
client of a contract rather than of a guess: a script polling the snapshot,
another tool aggregating several programs, or a test asserting on what a page
will render.

## Measuring a stage

`Accounted` is `Watching` plus what the process spent while the handler ran.
Two functions rather than one with an argument to ignore, because the cost is
real: two reads of `runtime/metrics` per request.

**The window is the handler's, not the request's.** A route decodes, runs the
handler and encodes; what these wrap is the handler. A route whose response is
expensive to encode looks cheaper here than it is.

**Go's CPU accounting does not move over a fast window**, so a handler
answering in twenty microseconds reports zero CPU. The allocation counters are
exact and do not have this problem.

Inside a handler, `process.Measured` gives a stage its own span, name and
account in one call — which is what makes a timeline worth drawing and a hot
path worth ranking:

```go
held := direct.Bind(bind, process.Measured(costs, "read", store.All()))
digest := direct.Bind(bind, process.Measured(costs, "digest", digesting(held)))
```

## The page

Panels: the process as gauges and charts; cost by route and stage; the fibers
and open spans with their ages; the traces; one trace on a timeline with a
details panel; the hot paths; the measurements; and the surface, whose openable
paths are links to the app.

**Traces are per trace, not per window.** Scaling every bar to the whole
reading made each one a hairline — a reading spans the minutes between requests
and a request spans microseconds. Choosing a trace scales its own spans to its
own duration, and the panel arrives on the newest trace that has something to
decompose rather than on the newest of all.

**Hot paths** aggregate every span in the window by the chain of names that
reaches it and rank by self time. Summed across traces, so a stage that is fast
once and runs two hundred times ranks above one that is slow once — which is
what "where does the time go" is actually asking.

**Events are marks on their span's bar**, placed by `Event.AtMicros`, and the
details panel lists them with their offsets. A retry's attempts appear on the
stage that retried rather than as a gap in the route's total.

Charts are [uPlot](https://github.com/leeoniya/uPlot)'s: axes, a cursor that
reads every series under the pointer, and a legend. It is **vendored** —
checked in under `inspect/assets`, served by the inspector, its MIT licence
beside it and served too. Nothing is fetched from off the process: no CDN, no
font, no third party at run time. A tool that only works where the internet is
reachable is not a debugging tool, and an architecture test asserts it fetches
nothing off-process.

The timeline is drawn here rather than by the library, because no chart library
draws a span waterfall — rows of nested bars on a shared axis with events
marked on them — so it carries its own axis and its own details panel.

The page polls, pauses while the tab is hidden, and says so in the header when
a read fails: an inspector that silently stops refreshing looks exactly like a
program that has stopped doing anything.

## Deliberately absent

**A push protocol.** Effect's devtools are a client and a server exchanging
spans, span events and metric snapshots over a transport, because the tool
reading them is an editor extension outside the process. This inspector is
*inside* the program, so shipping the events somewhere becomes unnecessary and
a protocol is what it would cost. The cases that trade loses are real and worth
naming: a program without an HTTP surface has nothing to mount this on, and a
program that has already crashed cannot be asked anything.

**Authentication.** The inspector answers whatever reaches it. Mounting it is a
decision about a surface, and who may reach that surface is the program's own
middleware to say — a tool that shipped its own answer would be one more thing
to configure and one more place for it to be wrong.

**Writing anything.** It reads. Nothing here interrupts a fiber, closes a
scope, or changes what the program is doing: a tool that could break the
program it is watching is a tool nobody leaves installed.
