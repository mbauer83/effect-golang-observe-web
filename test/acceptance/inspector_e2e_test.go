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

// serveExample starts the example program with the inspector mounted and returns
// a client for it. It stops when the test ends.
func serveExample(t *testing.T) *web.Client {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	window, err := observe.NewRecent(256)
	if err != nil {
		t.Fatal(err)
	}
	series, err := process.NewSeries(8)
	if err != nil {
		t.Fatal(err)
	}
	// The route names and the report's stage names: the vocabulary the
	// aggregate and the account are both bounded by.
	names := append(inspect.Names(declarations()), inspected.Stages...)
	telemetry := &inspect.Telemetry{
		Spans:     trace.NewSpans(),
		Fibers:    trace.NewFibers(),
		Window:    window,
		Collector: metrics.NewCollector(metrics.NewVocabulary(names...)),
		Series:    series,
		Costs:     process.NewCosts(names...),
	}
	runtime, err := effect.NewRuntime(
		// Unbuffered on purpose: a test that had to wait for a queue to drain
		// would be a test of the queue.
		effect.WithObserver(observe.Fanout(
			telemetry.Spans, telemetry.Fibers, telemetry.Collector, telemetry.Window)),
		effect.WithDebugTracking(),
	)
	if err != nil {
		t.Fatal(err)
	}
	telemetry.LiveWork = runtime.LiveWork

	store, built := runtime.Run(context.Background(), effect.Unit{},
		inspected.NewStore(inspected.Note{Title: "First", Body: "a note"})).Value()
	if !built {
		t.Fatal("the store could not be built")
	}
	surface, err := inspected.Surface(store, telemetry)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := inspected.Boundary(runtime)
	if err != nil {
		t.Fatal(err)
	}

	ctx, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(ctx, effect.Unit{}, inspected.Serve(listener, boundary, surface))
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

func declarations() []web.Declaration {
	return []web.Declaration{
		inspected.ListNotes.Declaration(),
		inspected.AddNote.Declaration(),
		inspected.Summarise.Declaration(),
		inspected.FindNote.Declaration(),
	}
}

// fetch calls one path and reads the whole response.
func fetch(t *testing.T, client *web.Client, method string, path string) web.ClientResponse {
	t.Helper()
	exit := effect.Run(context.Background(), effect.Unit{},
		web.Fetch[effect.Unit](client, method, path, web.ClientRequest{}))
	response, ok := exit.Value()
	if !ok {
		cause, _ := exit.Cause()
		t.Fatalf("calling %s %s: %v", method, path, cause)
	}
	return response
}

// readSnapshot reads the inspector's snapshot through its own description.
func readSnapshot(t *testing.T, client *web.Client) inspect.Snapshot {
	t.Helper()
	response := fetch(t, client, http.MethodGet, inspect.DefaultAt+"/snapshot")
	if response.Status != http.StatusOK {
		t.Fatalf("the inspector answered %d", response.Status)
	}
	snapshot, err := inspect.Read(response.Entity)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestEveryRequestBecomesASpanNamedForItsRoutePattern(t *testing.T) {
	client := serveExample(t)
	fetch(t, client, http.MethodGet, "/notes")
	fetch(t, client, http.MethodGet, "/notes/First")
	fetch(t, client, http.MethodGet, "/notes/Missing")

	snapshot := readSnapshot(t, client)
	counts := map[string]int{}
	for _, span := range snapshot.Trace {
		counts[span.Name]++
	}
	// The pattern and never the path: a series per title would be a series
	// per request, which is the failure metrics exists to avoid.
	if counts["GET /notes/{title}"] != 2 {
		t.Fatalf("expected both title requests under one name, got %v", counts)
	}
	if counts["GET /notes"] != 1 {
		t.Fatalf("expected the listing named for its route, got %v", counts)
	}
	for name := range counts {
		if strings.Contains(name, "First") || strings.Contains(name, "Missing") {
			t.Fatalf("a concrete path became a span name: %q", name)
		}
	}
	// The one that refused is visible as such, which is the question a trace
	// gets opened for -- and with the phases named, the failure is on the
	// route and on the phase it came out of rather than on the route alone.
	failures := map[string]int{}
	for _, span := range snapshot.Trace {
		if span.Status == string(effect.EventStatusFailure) {
			failures[span.Name]++
		}
	}
	if failures["GET /notes/{title}"] != 1 {
		t.Fatalf("expected the refused request's route to show as failed, got %v", failures)
	}
	if failures["handling"] != 1 {
		t.Fatalf("expected the failure attributed to the handling phase, got %v", failures)
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
	snapshot := readSnapshot(t, serveExample(t))

	found := map[string]Route{}
	for _, route := range snapshot.Routes {
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
