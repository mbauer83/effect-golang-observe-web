package inspected

// The surface, with the inspector on it.

import (
	"net"
	"net/http"

	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Endpoints are the program's own declarations, as values, so the same three
// dispatch a request, project into a contract, and name the spans.
var (
	// ListNotes reads every note.
	ListNotes = web.GET("/notes", web.Nothing(),
		web.Returns(http.StatusOK, NotesSchema)).
		Summary("List the notes")

	// AddNote keeps one, identified by its title.
	AddNote = web.POST("/notes", web.Entity(NoteSchema),
		web.Returns(http.StatusCreated, NoteSchema)).
		Summary("Add a note").
		Failing(http.StatusConflict, "a note with that title is held already")

	// Summarise is the route with something to decompose: four measured
	// stages, one of which retries.
	Summarise = web.GET("/notes/report", web.Nothing(),
		web.Returns(http.StatusOK, ReportSchema)).
		Summary("Summarise the notes in four measured stages")

	// FindNote reads one by title.
	FindNote = web.GET("/notes/{title}",
		web.PathParam("title", NoteTitle).Documented("the title to look for"),
		web.Returns(http.StatusOK, NoteSchema)).
		Summary("Find a note by title").
		Failing(http.StatusNotFound, "no note with that title is held")
)

// Surface assembles the program's routes with the inspector's.
//
// One surface, so one matcher dispatches everything: the inspector is not a
// second server on a second port that has to be found, configured and secured
// separately.
//
// The routes are declared and handled exactly as they would be without any of
// this -- web.Handle, nothing else -- and the observation is one line further
// down. That is the point: a program does not get written differently because
// somebody wants to watch it.
func Surface(store *Store, watched inspect.Watched) (web.Routes[effect.Unit, Refusal], error) {
	mine := []web.Route[effect.Unit, Refusal]{
		web.Handle(ListNotes, func(effect.Unit) storing[[]Note] { return store.All() }),
		web.Handle(AddNote, store.Add),
		// Declared before FindNote, because /notes/report and
		// /notes/{title} could both match a request for the first -- and a
		// literal segment beats a capture, so the matcher prefers this one
		// whatever the order. Stated in this order anyway, so a reader of the
		// list is not left working that out.
		web.Handle(Summarise,
			func(effect.Unit) storing[Report] { return Reported(store, watched.Costs) }),
		web.Handle(FindNote, store.Find),
	}
	described, err := web.NewRoutes(mine...)
	if err != nil {
		return web.Routes[effect.Unit, Refusal]{}, err
	}
	watched.Surface = described.Declarations

	inspecting, err := inspect.Routes[effect.Unit, Refusal](watched, inspect.DefaultAt)
	if err != nil {
		return web.Routes[effect.Unit, Refusal]{}, err
	}
	assembled, err := web.NewRoutes(append(mine, inspecting...)...)
	if err != nil {
		return web.Routes[effect.Unit, Refusal]{}, err
	}
	// The inspector's routes are observed like the program's, deliberately.
	// A tool's own load is part of what a program is doing, and hiding it
	// would be the one measurement a reader could not check.
	// The whole of the integration: one setting on the surface. Not applying
	// it is how a program turns observation off, which a caller can decide
	// from a flag without assembling anything differently.
	// Measuring as well, so a trace shows decoding and encoding beside the
	// handler and each of them says what it allocated: a large document to
	// unmarshal is real time, and one bar for all three could not say which
	// of them a slow request spent it in.
	return assembled.
		Measuring(inspect.Sampling(watched.Costs)).
		Wrapping(inspect.Observing[effect.Unit, Refusal](watched.Costs)), nil
}

// Serve runs the surface on a listener until its scope closes.
func Serve(
	listener net.Listener,
	boundary web.Adapter[effect.Unit, Refusal],
	surface web.Routes[effect.Unit, Refusal],
) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
	return effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, web.Fault, effect.Unit] {
		return web.ServeWith[effect.Unit](scope,
			web.Settings{Listener: listener},
			boundary.Handler(surface.Handler()),
		).FlatMap(web.Await[effect.Unit])
	})
}

// Boundary maps the program's refusals to statuses, at the boundary and
// nowhere else.
func Boundary(runtime *effect.Runtime) (web.Adapter[effect.Unit, Refusal], error) {
	return web.NewAdapter(runtime, effect.Unit{},
		func(refusal Refusal) web.Response {
			return web.Text(StatusFor(refusal), refusal.Error())
		})
}
