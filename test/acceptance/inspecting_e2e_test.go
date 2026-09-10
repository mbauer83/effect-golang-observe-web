package acceptance

// The inspector inside the program it inspects, over a real socket.
//
// What only this can answer: that the surface assembles with the program's own
// routes rather than beside them, that a request becomes a span named for its
// route pattern, that the snapshot the page reads decodes through the
// description the inspector publishes, and that the page is served at all.

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-observe-web/examples/inspected"
	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-observe/metrics"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// inspecting starts the example program with the inspector mounted and returns
// a client for it. It stops when the test ends.
func inspecting(t *testing.T) *web.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	window, err := observe.Keep(256)
	if err != nil {
		t.Fatal(err)
	}
	series, err := process.Keep(8)
	if err != nil {
		t.Fatal(err)
	}
	// The route names and the report's stage names: the vocabulary the
	// aggregate and the account are both bounded by.
	named := append(inspect.Names(declared()), inspected.Stages...)
	watched := &inspect.Watched{
		Running:   trace.Watch(),
		Fibers:    trace.WatchFibers(),
		Window:    window,
		Collected: metrics.Collect(metrics.Naming(named...)),
		Series:    series,
		Costs:     process.Accounting(named...),
	}
	runtime, err := effect.NewRuntime(
		// Unbuffered on purpose: a test that had to wait for a queue to drain
		// would be a test of the queue.
		effect.WithObserver(observe.Fanout(
			watched.Running, watched.Fibers, watched.Collected, watched.Window)),
		effect.WithDebugTracking(),
	)
	if err != nil {
		t.Fatal(err)
	}
	watched.Owned = runtime.LiveWork

	store, built := runtime.Run(context.Background(), effect.Unit{},
		inspected.NewStore(inspected.Note{Title: "First", Body: "a note"})).Value()
	if !built {
		t.Fatal("the store could not be built")
	}
	surface, err := inspected.Surface(store, watched)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := inspected.Boundary(runtime)
	if err != nil {
		t.Fatal(err)
	}

	serving, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(serving, effect.Unit{}, inspected.Serve(listener, boundary, surface))
	}()
	t.Cleanup(func() {
		stop()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("the server did not stop within five seconds")
		}
	})
	return web.Dial(http.DefaultClient, "http://"+listener.Addr().String())
}

func declared() []web.Declaration {
	return []web.Declaration{
		inspected.ListNotes.Declaration(),
		inspected.AddNote.Declaration(),
		inspected.Summarise.Declaration(),
		inspected.FindNote.Declaration(),
	}
}

// fetched calls one path and reads the whole response.
func fetched(t *testing.T, client *web.Client, method string, path string) web.Received {
	t.Helper()
	exit := effect.Run(context.Background(), effect.Unit{},
		web.Fetch[effect.Unit](client, method, path, web.Requesting{}))
	received, ok := exit.Value()
	if !ok {
		cause, _ := exit.Cause()
		t.Fatalf("calling %s %s: %v", method, path, cause)
	}
	return received
}

// taken reads the inspector's snapshot through its own description.
func taken(t *testing.T, client *web.Client) inspect.Snapshot {
	t.Helper()
	received := fetched(t, client, http.MethodGet, inspect.DefaultAt+"/snapshot")
	if received.Status != http.StatusOK {
		t.Fatalf("the inspector answered %d", received.Status)
	}
	read, err := inspect.Read(received.Entity)
	if err != nil {
		t.Fatal(err)
	}
	return read
}

func TestEveryRequestBecomesASpanNamedForItsRoutePattern(t *testing.T) {
	client := inspecting(t)
	fetched(t, client, http.MethodGet, "/notes")
	fetched(t, client, http.MethodGet, "/notes/First")
	fetched(t, client, http.MethodGet, "/notes/Missing")

	read := taken(t, client)
	named := map[string]int{}
	for _, span := range read.Trace {
		named[span.Name]++
	}
	// The pattern and never the path: a series per title would be a series
	// per request, which is the failure metrics exists to avoid.
	if named["GET /notes/{title}"] != 2 {
		t.Fatalf("expected both title requests under one name, got %v", named)
	}
	if named["GET /notes"] != 1 {
		t.Fatalf("expected the listing named for its route, got %v", named)
	}
	for name := range named {
		if strings.Contains(name, "First") || strings.Contains(name, "Missing") {
			t.Fatalf("a concrete path became a span name: %q", name)
		}
	}
	// The one that refused is visible as such, which is the question a trace
	// gets opened for -- and with the phases named, the failure is on the
	// route and on the phase it came out of rather than on the route alone.
	failed := map[string]int{}
	for _, span := range read.Trace {
		if span.Status == string(effect.EventStatusFailure) {
			failed[span.Name]++
		}
	}
	if failed["GET /notes/{title}"] != 1 {
		t.Fatalf("expected the refused request's route to show as failed, got %v", failed)
	}
	if failed["handling"] != 1 {
		t.Fatalf("expected the failure attributed to the handling phase, got %v", failed)
	}
}

func TestTheSurfaceBeingServedReachesTheWire(t *testing.T) {
	// What a runtime-level tool cannot do, and the reason this module is a web
	// module: the declarations that dispatch a request are the ones shown, and
	// they cross a real socket to get here.
	//
	// Which routes those are is the program's choice -- the example reports
	// its own and not the inspector's -- and the inspector reports whatever it
	// is given, which the unit suite states directly.
	read := taken(t, inspecting(t))

	found := map[string]Route{}
	for _, route := range read.Routes {
		found[route.Method+" "+route.Path] = route
	}
	listing, present := found["GET /notes"]
	if !present {
		t.Fatalf("expected the listing in the surface, got %v", found)
	}
	if listing.Status != http.StatusOK || listing.Summary != "List the notes" {
		t.Fatalf("expected the declaration as declared, got %+v", listing)
	}
	if _, present := found["GET /notes/{title}"]; !present {
		t.Fatalf("expected the pattern and not a path, got %v", found)
	}
}

// Route is the wire shape, aliased so the assertions above read as prose.
type Route = inspect.Route
