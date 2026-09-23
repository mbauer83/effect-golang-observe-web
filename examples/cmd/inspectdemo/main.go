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

	"github.com/mbauer83/effect-golang-observe-web/examples/notebook"
	"github.com/mbauer83/effect-golang-observe-web/inspect"
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
	telemetry, observer, err := newTelemetry()
	if err != nil {
		fail(err)
	}
	runtime, err := effect.NewRuntime(
		effect.WithObserver(observer),
		effect.WithDebugTracking(),
	)
	if err != nil {
		fail(err)
	}
	telemetry.LiveWork = runtime.LiveWork

	store, built := runtime.Run(context.Background(), effect.Unit{},
		notebook.NewStore(notebook.Note{Title: "First", Body: "a note"})).Value()
	if !built {
		fail(errors.New("the store could not be built"))
	}
	surface, err := notebook.Surface(store, telemetry)
	if err != nil {
		fail(err)
	}
	boundary, err := notebook.Boundary(runtime)
	if err != nil {
		fail(err)
	}

	ctx, stop := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.Run(ctx, effect.Unit{}, notebook.Serve(listener, boundary, surface))
	}()
	fmt.Printf("inspected: listening on %s, inspector at %s%s\n",
		base, base, inspect.DefaultAt)

	exercise(base)
	report(base)

	if *serve {
		fmt.Printf("\nserving. open %s%s, or send an interrupt to stop\n",
			base, inspect.DefaultAt)
		interrupt, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		<-interrupt.Done()
	}

	stop()
	<-stopped
	if cleanup := runtime.Close(context.Background()); !cleanup.IsEmpty() {
		fmt.Printf("shutdown: %s\n", cleanup)
	}
}

// newTelemetry assembles the telemetry the inspector reads.
//
// The vocabulary is the interesting part and the only part left: the
// program's routes, the report's stages, the inspector's own routes, and the
// phases each request is broken into. Its own too, because a route left out
// of the vocabulary has its traffic lumped in with everything undeclared --
// which is how five hundred of the inspector's own spans came to sit in one
// bucket called "other". Its load is worth seeing; it is worth seeing as its
// own.
//
// Classes rather than scalars, because the size buckets are what this demo has
// to show: reading them costs about twenty nanoseconds more, and keeping them
// is a set of classes per name.
func newTelemetry() (*inspect.Telemetry, effect.Observer, error) {
	names := append(inspect.Names(declarations()), notebook.Stages...)
	names = append(names, inspect.Names(inspectorDeclarations())...)
	names = append(names, web.PhaseNames()...)
	return inspect.NewTelemetry(inspect.TelemetryConfig{
		Names:  names,
		Recent: 512,
		Detail: inspect.Classes,
	})
}

// declarations are the program's own routes, which is where the metric
// vocabulary comes from.
func declarations() []web.Declaration {
	return []web.Declaration{
		notebook.ListNotes.Declaration(),
		notebook.AddNote.Declaration(),
		notebook.Summarise.Declaration(),
		notebook.FindNote.Declaration(),
	}
}

// inspectorDeclarations are the inspector's own routes, for the vocabulary. Its handlers
// need a Telemetry and its declarations do not, so an empty one is enough to ask
// what it serves.
func inspectorDeclarations() []web.Declaration {
	routes, err := inspect.Routes[effect.Unit, notebook.Refusal](
		&inspect.Telemetry{}, inspect.DefaultAt)
	if err != nil {
		fail(err)
	}
	return web.DeclarationsOf(routes...)
}

// observing is the one observer a runtime takes: the live views inline,
// because "what is running now" must not be answered from a backlog, and the
// window and the aggregate behind a queue, because neither is asked often
// enough to be worth paying for on the observed fiber.

func fail(err error) {
	fmt.Fprintf(os.Stderr, "inspectdemo: %v\n", err)
	os.Exit(1)
}
