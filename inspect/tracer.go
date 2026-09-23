package inspect

// Making a program's traffic visible, without changing how the program is
// written.

import (
	"log/slog"

	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Tracer is the setting that makes a surface's traffic visible: every route
// becomes a span named for itself, annotated with its method and pattern.
//
// Applied once to the assembled surface and nowhere else:
//
//	surface, err := web.NewRoutes(routes...)
//	surface = surface.WithMiddleware(inspect.Tracer(costs))
//
// Which is the whole integration. Nothing about how the routes are declared or
// handled changes, so observation cannot be forgotten one route at a time --
// and turning it off is not applying it, which a caller can decide from a flag
// at start-up.
//
// A runtime brackets what a program tells it to bracket, so without this a
// handler that opens no span of its own contributes nothing to a trace: a
// request appears as a fiber that ran and nothing more.
//
// The name is the method and the route's *pattern* -- "GET /books/{title}" --
// and never the path that was asked for. A concrete path is an unbounded
// value, and the rule that forbids it as a metric label forbids it as a span
// name: a series per title is a series per request.
func Tracer[R, E any](costs *process.Costs) web.RouteMiddleware[R, E] {
	return func(
		declaration web.Declaration,
		handler web.Handler[R, E],
	) web.Handler[R, E] {
		name := NameOf(declaration)
		method := slog.String("method", declaration.Method)
		route := slog.String("route", declaration.Path)
		return func(request web.Request) effect.Effect[R, E, web.Response] {
			// Annotate outside WithSpan, not inside. Metadata supplied inside
			// a span applies to the work within it and not to the span's own
			// start and end, so annotating inside put the method and the route
			// on nothing a reader of the trace can see. Measured, not
			// reasoned: the same span reports [] one way round and
			// [method=..., route=...] the other.
			return process.Track(costs, name, handler(request)).
				WithName(name).
				WithSpan(name).
				Annotate(method, route)
		}
	}
}

// NameOf is the span name a route is observed under.
func NameOf(declaration web.Declaration) string {
	return declaration.Method + " " + declaration.Path
}

// Names are the span names a surface's routes are observed under.
//
// It is what a caller passes to metrics.NewVocabulary and process.NewCosts, which
// closes the loop: the vocabulary of a bounded aggregate comes from the same
// declarations that dispatch the requests, so a route added to the surface is
// measured without anybody remembering to add its name in a second place.
//
//	collector := metrics.NewCollector(metrics.NewVocabulary(inspect.Names(surface.Declarations())...))
func Names(declarations []web.Declaration) []string {
	names := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		names = append(names, NameOf(declaration))
	}
	return names
}

// Sampler measures a surface's phases into the account, so decoding and
// encoding say what they allocated and not only how long they took.
//
//	surface = surface.WithPhaseSampler(inspect.Sampler(telemetry.Costs)).
//	    WithMiddleware(inspect.Tracer[Env, Refusal](telemetry.Costs))
//
// The transports name their phases and cannot measure them -- they depend on
// nothing that reads a counter -- so the naming is theirs and the measuring is
// here. Which is the same division as everywhere else in this package: the
// program says what its parts are, and this says what they cost.
//
// The figures carry the caveat every figure in this package carries: Go reports
// no per-goroutine allocation, so a phase's account is what the *process*
// allocated while that phase ran, summed over its runs. On a surface serving
// one request at a time that is the phase; on a busy one it includes whatever
// else was running, and the account says how many runs it is averaged over so
// a reader can judge it.
func Sampler(costs *process.Costs) web.PhaseSampler {
	if costs == nil {
		return nil
	}
	if !costs.KeepsSizes() {
		return func(phase string) func() {
			before := process.Read()
			return func() {
				costs.Record(phase, process.Diff(before, process.Read()))
			}
		}
	}
	// The account keeps the sizes, so the histogram is sampled at both ends
	// too. Two paths rather than one that always samples it, for the reason
	// Track has two: an account that does not keep the detail should not pay
	// to gather it.
	return func(phase string) func() {
		before, sizesBefore := process.Read(), process.ReadSizes()
		return func() {
			costs.RecordSpread(phase,
				process.Diff(before, process.Read()),
				process.DiffSizes(sizesBefore, process.ReadSizes()))
		}
	}
}
