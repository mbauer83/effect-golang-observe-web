// Command inspectdemo runs the inspected program with the inspector mounted,
// exercises it, and reports what the inspector answers.
//
// It exists so the tooling is demonstrably a program and not a diagram, and
// because the page and the snapshot behind it are read by people: a view
// nobody has looked at is a view nobody knows is legible.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"

	"github.com/mbauer83/effect-golang-observe-web/examples/inspected"
	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-observe/metrics"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// serve keeps the program running after the report, so the page can be opened.
// A flag rather than a second command: the program, the traffic and the
// inspector are the same three things either way.
var serve = flag.Bool("serve", false, "keep serving, so the inspector's page can be opened")

func main() {
	flag.Parse()
	address := "127.0.0.1:0"
	if *serve {
		// A stated port when a person is going to type it.
		address = "127.0.0.1:8099"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fail(err)
	}
	base := "http://" + listener.Addr().String()

	// The telemetry, assembled: the live views inline, the window and the
	// aggregate behind a queue. The vocabulary comes from the routes
	// themselves, so a route added to the surface is measured without anybody
	// remembering it in a second place.
	watched, err := watching()
	if err != nil {
		fail(err)
	}
	runtime, err := effect.NewRuntime(
		effect.WithObserver(observing(watched)),
		effect.WithDebugTracking(),
	)
	if err != nil {
		fail(err)
	}
	watched.Owned = runtime.LiveWork

	store, built := runtime.Run(context.Background(), effect.Unit{},
		inspected.NewStore(inspected.Note{Title: "First", Body: "a note"})).Value()
	if !built {
		fail(errors.New("the store could not be built"))
	}
	surface, err := inspected.Surface(store, watched)
	if err != nil {
		fail(err)
	}
	boundary, err := inspected.Boundary(runtime)
	if err != nil {
		fail(err)
	}

	serving, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(serving, effect.Unit{}, inspected.Serve(listener, boundary, surface))
	}()
	fmt.Printf("inspected: listening on %s, inspector at %s%s\n",
		base, base, inspect.DefaultAt)

	exercise(base)
	report(base)

	if *serve {
		fmt.Printf("\nserving. open %s%s, or send an interrupt to stop\n",
			base, inspect.DefaultAt)
		waiting, done := signal.NotifyContext(context.Background(), os.Interrupt)
		defer done()
		<-waiting.Done()
	}

	stop()
	<-stopped
	if cleanup := runtime.Close(context.Background()); !cleanup.IsEmpty() {
		fmt.Printf("shutdown: %s\n", cleanup)
	}
}

// watching assembles the telemetry the inspector reads.
func watching() (inspect.Watched, error) {
	window, err := observe.Keep(512)
	if err != nil {
		return inspect.Watched{}, err
	}
	// Room for a few minutes of chart at a couple of seconds a refresh.
	series, err := process.Keep(180)
	if err != nil {
		return inspect.Watched{}, err
	}
	// The vocabulary a bounded aggregate and a cost account are keyed by: the
	// program's routes, the report's stages, and the inspector's own routes.
	//
	// Its own too, because the inspector is observed like everything else and
	// a route left out of the vocabulary has its traffic lumped in with
	// everything undeclared -- which is how five hundred of its own spans came
	// to sit in one bucket called "other". Its load is worth seeing; it is
	// worth seeing as its own.
	named := append(inspect.Names(declarations()), inspected.Stages...)
	named = append(named, inspect.Names(inspecting())...)
	// And the phases, because the surface is detailing them: three spans per
	// request left undeclared are three spans per request in one bucket with
	// no name on it, which is how the biggest thing in the aggregate came to
	// be "other".
	named = append(named, web.PhaseNames()...)
	return inspect.Watched{
		Running:   trace.Watch(),
		Fibers:    trace.WatchFibers(),
		Window:    window,
		Collected: metrics.Collect(metrics.Naming(named...)),
		Series:    series,
		// Sizing rather than Accounting, because the size classes are what
		// this demo has to show: reading them costs about twenty nanoseconds
		// more than the scalars, and keeping them is a set of classes per
		// name.
		Costs: process.Sizing(named...),
	}, nil
}

// declarations are the program's own routes, which is where the metric
// vocabulary comes from.
func declarations() []web.Declaration {
	return []web.Declaration{
		inspected.ListNotes.Declaration(),
		inspected.AddNote.Declaration(),
		inspected.Summarise.Declaration(),
		inspected.FindNote.Declaration(),
	}
}

// inspecting are the inspector's own routes, for the vocabulary. Its handlers
// need a Watched and its declarations do not, so an empty one is enough to ask
// what it serves.
func inspecting() []web.Declaration {
	routes, err := inspect.Routes[effect.Unit, inspected.Refusal](
		inspect.Watched{}, inspect.DefaultAt)
	if err != nil {
		fail(err)
	}
	return web.DeclarationsOf(routes...)
}

// observing is the one observer a runtime takes: the live views inline,
// because "what is running now" must not be answered from a backlog, and the
// window and the aggregate behind a queue, because neither is asked often
// enough to be worth paying for on the observed fiber.
func observing(watched inspect.Watched) effect.Observer {
	queued, err := observe.Buffer(
		observe.Fanout(watched.Collected, watched.Window), 2048, observe.DropOldest)
	if err != nil {
		fail(err)
	}
	return observe.Fanout(watched.Running, watched.Fibers, queued)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "inspectdemo: %v\n", err)
	os.Exit(1)
}
