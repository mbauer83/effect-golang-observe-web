package inspect

// The inspector as a mountable web surface.

import (
	"net/http"
	"strings"
	"time"

	"github.com/mbauer83/effect-golang-web/openapi"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// DefaultAt is where the inspector mounts when a caller states no path.
const DefaultAt = "/inspect"

// Surface is the inspector's routes, mounted under at.
//
// A web.Routes and not a server, because the inspector belongs inside the
// program it inspects: on the same listener, in the same runtime, holding the
// same telemetry. A separate process would need the events shipped to it, and
// shipping them is the thing this module exists to make unnecessary.
//
// Three endpoints. The page is what a person opens; the snapshot is what the
// page reads and what a script reads; the contract is what says so.
func Surface[R, E any](watched *Watched, at string) (web.Routes[R, E], error) {
	mounted, err := Routes[R, E](watched, at)
	if err != nil {
		return web.Routes[R, E]{}, err
	}
	return web.NewRoutes(mounted...)
}

// Routes are the inspector's routes, for mounting inside a program's own
// surface.
//
// The list rather than an assembled Routes, because a program serves one
// surface: its own routes and these, assembled together, so one matcher
// dispatches everything and an ambiguity between them is reported at start-up
// like any other.
//
//	surface, err := web.NewRoutes(append(mine, inspecting...)...)
func Routes[R, E any](watched *Watched, at string) ([]web.Route[R, E], error) {
	mounted := mountedAt(at)
	own := []web.Route[R, E]{
		page[R, E](mounted),
		snapshot[R, E](watched, mounted),
		asset[R, E](mounted, "/uplot.js", "text/javascript; charset=utf-8",
			"The chart library the page draws with", Script),
		asset[R, E](mounted, "/uplot.css", "text/css; charset=utf-8",
			"The chart library's stylesheet", Stylesheet),
		asset[R, E](mounted, "/uplot.LICENSE", "text/plain; charset=utf-8",
			"The chart library's licence", Licence),
	}

	// Assembled once to have the declarations the contract is projected from,
	// and the contract route added afterwards. The contract describes the
	// inspector and not the route serving it, which is the ordinary
	// arrangement -- a reader already knows where the document is.
	described, err := web.NewRoutes(own...)
	if err != nil {
		return nil, Fault{Doing: "assembling the inspector", Err: err}
	}
	contract, err := describe(described, mounted)
	if err != nil {
		return nil, err
	}
	return append(own, published[R, E](contract, mounted)), nil
}

// mountedAt normalises the mount point, so a caller may write "/debug",
// "/debug/" or "" and get the same surface.
func mountedAt(at string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(at), "/")
	if trimmed == "" {
		return DefaultAt
	}
	if !strings.HasPrefix(trimmed, "/") {
		return "/" + trimmed
	}
	return trimmed
}

// page serves the inspector itself: one document, no external assets.
//
// ReturnsRaw because the entity is not built from a value. There is no
// description of an HTML page to read it through, which is exactly what a
// content entry with no schema means.
func page[R, E any](at string) web.Route[R, E] {
	return web.Handle(
		web.GET(at, web.Nothing(), web.ReturnsRaw(http.StatusOK, "text/html; charset=utf-8")).
			Summary("The inspector").
			Describe("One document, which reads the snapshot below."),
		func(effect.Unit) effect.Effect[R, E, []byte] {
			return effect.For[R, E]().Succeed(Page())
		},
	)
}

// snapshot serves one reading, described.
func snapshot[R, E any](watched *Watched, at string) web.Route[R, E] {
	return web.Handle(
		web.GET(at+"/snapshot", web.Nothing(),
			web.Returns(http.StatusOK, SnapshotSchema)).
			Summary("One reading of the program's own telemetry").
			Describe("The spans open now, the recent events as a tree, the bounded "+
				"measurements, and the surface being served. Read one after another "+
				"rather than at one instant, because a running program does not stop "+
				"between them."),
		func(effect.Unit) effect.Effect[R, E, Snapshot] {
			return effect.For[R, E]().Suspend(func() effect.Effect[R, E, Snapshot] {
				return effect.For[R, E]().Succeed(watched.Take(time.Now()))
			}).WithName("take-snapshot")
		},
	)
}

// asset serves one checked-in file beside the page.
//
// Served rather than inlined, so the page stays a document a reviewer can
// read: fifty kilobytes of minified library pasted into it would make the part
// worth reading unfindable. And served from the mount point rather than a
// fixed path, so two programs on one host do not disagree about what
// /uplot.js is.
func asset[R, E any](
	at string,
	name string,
	mediaType string,
	summary string,
	content func() []byte,
) web.Route[R, E] {
	return web.Handle(
		web.GET(at+name, web.Nothing(), web.ReturnsRaw(http.StatusOK, mediaType)).
			Summary(summary),
		func(effect.Unit) effect.Effect[R, E, []byte] {
			return effect.For[R, E]().Succeed(content())
		},
	)
}

// published serves the inspector's own contract.
//
// Projected from the same declarations that dispatch the requests, so a script
// reading the snapshot has a described shape to read it as rather than a
// guess. A tool whose API is guessed at is one somebody writes a client for
// twice.
func published[R, E any](contract []byte, at string) web.Route[R, E] {
	return web.Handle(
		web.GET(at+"/openapi.json", web.Nothing(),
			web.ReturnsRaw(http.StatusOK, "application/json")).
			Summary("The contract the inspector is served from"),
		func(effect.Unit) effect.Effect[R, E, []byte] {
			return effect.For[R, E]().Succeed(contract)
		},
	)
}

func describe[R, E any](surface web.Routes[R, E], at string) ([]byte, error) {
	rendered, err := openapi.Describe(
		openapi.Info{
			Title:       "effect-golang inspector",
			Version:     "1.0.0",
			Description: "A program's own telemetry, and the surface it serves.",
		},
		surface.Declarations(),
	).Render()
	if err != nil {
		return nil, Fault{Doing: "describing the inspector at " + at, Err: err}
	}
	return rendered, nil
}
