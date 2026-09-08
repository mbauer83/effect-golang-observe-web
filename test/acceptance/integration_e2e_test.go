package acceptance

// The integration itself: one setting on the surface, and nothing about how
// the program is written.
//
// The program's routes are declared and handled with plain web.Handle. What
// makes their traffic visible is a single Wrapping at assembly, so observation
// cannot be forgotten one route at a time -- and not applying it is how a
// program turns it off.

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-observe-web/examples/inspected"
	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// plainly serves the program's own routes, observed or not, and returns a
// client for them together with the window the events land in.
//
// Deliberately not the example's Surface: this is about what the setting does,
// so the routes here are declared with web.Handle and nothing else.
func plainly(t *testing.T, observed bool) (*web.Client, *observe.Recent) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	window, err := observe.Keep(256)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := effect.NewRuntime(
		effect.WithObserver(observe.Fanout(trace.Watch(), window)),
	)
	if err != nil {
		t.Fatal(err)
	}
	store, built := runtime.Run(context.Background(), effect.Unit{},
		inspected.NewStore(inspected.Note{Title: "First", Body: "a note"})).Value()
	if !built {
		t.Fatal("the store could not be built")
	}

	// Written as any program writes them.
	surface, err := web.NewRoutes(
		web.Handle(inspected.ListNotes,
			func(effect.Unit) effect.Effect[effect.Unit, inspected.Refusal, []inspected.Note] {
				return store.All()
			}),
		web.Handle(inspected.FindNote, store.Find),
	)
	if err != nil {
		t.Fatal(err)
	}
	// The one difference between an observed program and an unobserved one.
	if observed {
		surface = surface.Wrapping(
			inspect.Observing[effect.Unit, inspected.Refusal](process.Accounting()))
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
	return web.Dial(http.DefaultClient, "http://"+listener.Addr().String()), window
}

func TestOneSettingObservesEveryRouteOfASurfaceWrittenPlainly(t *testing.T) {
	client, window := plainly(t, true)
	fetched(t, client, http.MethodGet, "/notes")
	fetched(t, client, http.MethodGet, "/notes/First")

	assembled := trace.Assemble(window.Events())
	named := map[string]int{}
	for _, span := range assembled.Spans() {
		named[span.Name]++
	}
	// Both routes, each under its own pattern, from routes declared with
	// web.Handle and nothing else.
	if named["GET /notes"] != 1 || named["GET /notes/{title}"] != 1 {
		t.Fatalf("expected a span per route, got %v", named)
	}
	// Annotated with the method and the pattern, which is what a reader of a
	// trace needs and what a bounded label is made from.
	for _, span := range assembled.Spans() {
		held := map[string]string{}
		for _, attribute := range span.Attributes {
			held[attribute.Key] = attribute.Value.String()
		}
		if held["method"] != http.MethodGet || held["route"] == "" {
			t.Fatalf("expected %s annotated with its method and route, got %v",
				span.Name, held)
		}
	}
}

func TestNotApplyingTheSettingObservesNothing(t *testing.T) {
	// Which is how a program turns it off: a flag at start-up, not a
	// differently written surface.
	client, window := plainly(t, false)
	fetched(t, client, http.MethodGet, "/notes")
	fetched(t, client, http.MethodGet, "/notes/First")

	assembled := trace.Assemble(window.Events())
	if spans := assembled.Spans(); len(spans) != 0 {
		t.Fatalf("expected no spans from an unobserved surface, got %v", spans)
	}
	// The events still arrived -- the runtime is still observed -- so this is
	// the setting's absence and not a broken observer.
	if window.Seen() == 0 {
		t.Fatal("expected the runtime's own events even unobserved")
	}
}
