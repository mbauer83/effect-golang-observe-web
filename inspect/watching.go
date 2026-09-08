package inspect

// Making a request show up in a trace.

import (
	"log/slog"

	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Watching gives an endpoint its handler and makes the work a span named for
// the route.
//
// A drop-in for web.Handle. A runtime brackets what a program tells it to
// bracket, so a handler that opens no span of its own contributes nothing to a
// trace: a request would appear as a fiber that ran and nothing more. This is
// the one line that changes that for a whole surface.
//
// The name is the method and the route's *pattern* -- "GET /books/{title}" --
// and never the path that was asked for. A concrete path is an unbounded
// value, and the same rule that forbids it as a metric label forbids it as a
// span name: a series per title is a series per request, which is the failure
// mode metrics exists to avoid.
func Watching[R, E, In, Out any](
	endpoint web.Endpoint[In, Out],
	handle func(In) effect.Effect[R, E, Out],
) web.Route[R, E] {
	return Accounted[R, E](nil, endpoint, handle)
}

// Accounted is Watching, and also records what the process spent while the
// work ran.
//
// Two functions rather than one with an argument to ignore, because the cost
// of measuring is real: two reads of runtime/metrics per request. A program
// that wants its traffic in a trace and not its allocation in an account uses
// Watching and pays for neither.
//
// The account is keyed by the same route name the span is, so it is bounded by
// the surface. What it holds is process-wide over each request's window --
// concurrent requests are in each other's numbers -- and the field names in
// the account say so.
//
// The window is the handler's, not the request's. A Route decodes the request,
// runs the handler and encodes the response, and what this wraps is the
// handler: the span and the account both cover the work the application wrote
// and not the codecs around it. A route whose response is expensive to encode
// will look cheaper here than it is, which is worth knowing before drawing a
// conclusion from the number.
func Accounted[R, E, In, Out any](
	costs *process.Costs,
	endpoint web.Endpoint[In, Out],
	handle func(In) effect.Effect[R, E, Out],
) web.Route[R, E] {
	declared := endpoint.Declaration()
	name := NameOf(declared)
	return web.Handle(endpoint, func(input In) effect.Effect[R, E, Out] {
		// Annotate outside WithSpan, not inside. Metadata supplied inside a
		// span applies to the work within it and not to the span's own
		// start and end -- so annotating inside put the method and the route
		// on nothing a reader of the trace can see. Measured, not reasoned:
		// the same span with the same annotation reports [] one way round and
		// [method=..., route=...] the other.
		return process.Costing(costs, name, handle(input)).
			Named(name).
			WithSpan(name).
			Annotate(
				slog.String("method", declared.Method),
				slog.String("route", declared.Path),
			)
	})
}

// NameOf is the span name Watching gives a declaration.
func NameOf(declaration web.Declaration) string {
	return declaration.Method + " " + declaration.Path
}

// Names are the span names a surface's routes are watched under.
//
// It is what a caller passes to metrics.Naming, which closes the loop: the
// vocabulary of a bounded aggregate comes from the same declarations that
// dispatch the requests, so a route added to the surface is measured without
// anybody remembering to add its name in a second place.
//
//	collected := metrics.Collect(metrics.Naming(inspect.Names(surface.Declarations())...))
func Names(declarations []web.Declaration) []string {
	named := make([]string, 0, len(declarations))
	for _, declaration := range declarations {
		named = append(named, NameOf(declaration))
	}
	return named
}
