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
// separately. inspect.Watching in place of web.Handle is the whole of making
// the traffic visible -- every request becomes a span named for its route.
func Surface(store *Store, watched inspect.Watched) (web.Routes[effect.Unit, Refusal], error) {
	// Accounted rather than Watching, because this example is what the
	// inspector's cost panel is demonstrated with: it names each route's work
	// as a span and records what the process spent while it ran.
	mine := []web.Route[effect.Unit, Refusal]{
		inspect.Accounted(watched.Costs, ListNotes,
			func(effect.Unit) storing[[]Note] { return store.All() }),
		inspect.Accounted(watched.Costs, AddNote, store.Add),
		// Declared before FindNote, because /notes/report and
		// /notes/{title} could both match a request for the first -- and a
		// literal segment beats a capture, so the matcher prefers this one
		// whatever the order. Stated in this order anyway, so a reader of the
		// list is not left working that out.
		inspect.Accounted(watched.Costs, Summarise,
			func(effect.Unit) storing[Report] { return Reported(store, watched.Costs) }),
		inspect.Accounted(watched.Costs, FindNote, store.Find),
	}
	// The inspector is given the declarations of the program's own routes, and
	// not its own: what a person wants to see is the surface being served, and
	// the inspector is the thing looking rather than the thing looked at.
	described, err := web.NewRoutes(mine...)
	if err != nil {
		return web.Routes[effect.Unit, Refusal]{}, err
	}
	watched.Surface = described.Declarations

	inspecting, err := inspect.Routes[effect.Unit, Refusal](watched, inspect.DefaultAt)
	if err != nil {
		return web.Routes[effect.Unit, Refusal]{}, err
	}
	return web.NewRoutes(append(mine, inspecting...)...)
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
