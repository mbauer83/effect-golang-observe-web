package main

// Sending the traffic, and reading back what the inspector says about it.
//
// Its own file because it is the other half of the demo: main starts the
// program and this exercises it, which is the only way an inspector has
// anything to show.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// exercise sends the traffic the inspector then has something to show about.
func exercise(base string) {
	client := web.Dial(http.DefaultClient, base)
	for _, sending := range traffic() {
		if _, err := called(client, sending); err != nil {
			fmt.Printf("  %s: %v\n", sending.what, err)
		}
	}
}

type sending struct {
	what   string
	method string
	path   string
	body   []byte
}

func traffic() []sending {
	return []sending{
		{what: "list", method: http.MethodGet, path: "/notes"},
		{what: "add", method: http.MethodPost, path: "/notes",
			body: []byte(`{"title":"Second","body":"another"}`)},
		{what: "add again", method: http.MethodPost, path: "/notes",
			body: []byte(`{"title":"Second","body":"another"}`)},
		{what: "find", method: http.MethodGet, path: "/notes/Second"},
		{what: "find missing", method: http.MethodGet, path: "/notes/Missing"},
		{what: "summarise", method: http.MethodGet, path: "/notes/report"},
	}
}

func called(client *web.Client, one sending) (web.Received, error) {
	requesting := web.Requesting{Entity: one.body}
	if len(one.body) > 0 {
		requesting.MediaType = "application/json"
	}
	exit := effect.Run(context.Background(), effect.Unit{},
		web.Fetch[effect.Unit](client, one.method, one.path, requesting))
	received, ok := exit.Value()
	if !ok {
		cause, _ := exit.Cause()
		return web.Received{}, errors.New(cause.String())
	}
	return received, nil
}

// report reads the inspector's own snapshot, through the client, and prints
// what it says -- which is the same reading the page renders.
//
// Twice, with a pause: a reading is taken when a snapshot is, and a window
// needs two of them. One reading gives every gauge and no rate, which is what
// the page shows on its first refresh too.
func report(base string) {
	client := web.Dial(http.DefaultClient, base)
	if _, err := called(client, sending{
		method: http.MethodGet, path: inspect.DefaultAt + "/snapshot",
	}); err != nil {
		fail(err)
	}
	time.Sleep(300 * time.Millisecond)
	received, err := called(client, sending{
		method: http.MethodGet, path: inspect.DefaultAt + "/snapshot",
	})
	if err != nil {
		fail(err)
	}
	taken, err := inspect.Read(received.Entity)
	if err != nil {
		fail(err)
	}

	fmt.Printf("\nthe inspector, read at %s\n", taken.TakenAt)
	fmt.Printf("  the runtime owns %d fiber(s) and %d resource(s)\n",
		taken.Owned.Fibers, taken.Owned.Resources)
	fmt.Printf("  %d span(s) in the window, %d event(s) outside every span\n",
		len(taken.Trace), taken.LooseEvents)
	for _, span := range taken.Trace {
		fmt.Printf("  %s%-28s +%-10s %-10s %s\n",
			indent(span.Depth), span.Name,
			offset(span.StartMicros), micros(span.Micros), span.Status)
	}
	fmt.Println("\n  what was served")
	for _, route := range taken.Routes {
		fmt.Printf("    %-6s %-18s answers %d  %s\n",
			route.Method, route.Path, route.Status, route.Summary)
	}
	fmt.Printf("\n  the process, %s into the window\n",
		micros(taken.Process.OverMicros))
	fmt.Printf("    heap %s of a %s goal, %d objects, stacks %s\n",
		bytes(taken.Process.HeapBytes), bytes(taken.Process.GoalBytes),
		taken.Process.HeapObjects, bytes(taken.Process.StackBytes))
	fmt.Printf("    %d goroutine(s) on %d thread(s): %d running, %d runnable, %d waiting\n",
		taken.Process.Goroutines, taken.Process.Threads,
		taken.Process.Running, taken.Process.Runnable, taken.Process.Waiting)
	fmt.Printf("    allocated %s at %s/s, %.1f%% busy, %.1f%% of that collecting\n",
		bytes(taken.Process.AllocatedBytes), bytes(int64(taken.Process.BytesPerSecond)),
		100*taken.Process.Busy, 100*taken.Process.Collecting)

	if len(taken.Costs) > 0 {
		fmt.Println("\n  what the process spent while each route ran")
		fmt.Println("    (process-wide over each request's window, so concurrent work is in it too)")
		for _, cost := range taken.Costs {
			fmt.Printf("    %-24s x%-3d %8s per run, longest %s\n",
				cost.Name, cost.Times, bytes(cost.PerRunBytes),
				micros(cost.LongestMicros))
		}
	}

	fmt.Println("\n  what it measured, per route")
	for _, measured := range taken.Measurements {
		if measured.Kind != "span_ended" {
			continue
		}
		fmt.Printf("    %-24s %-14s x%d  median at most %s\n",
			measured.Operation, measured.Status, measured.Count,
			micros(measured.MedianMicros))
	}
}

func indent(depth int64) string {
	written := ""
	for range depth {
		written += "  "
	}
	return written
}

// micros renders a measurement, where zero means there was none.
// bytes renders a byte count the way a person reads one.
func bytes(value int64) string {
	switch {
	case value <= 0:
		return "-"
	case value < 1<<10:
		return fmt.Sprintf("%dB", value)
	case value < 1<<20:
		return fmt.Sprintf("%.1fKiB", float64(value)/(1<<10))
	case value < 1<<30:
		return fmt.Sprintf("%.1fMiB", float64(value)/(1<<20))
	}
	return fmt.Sprintf("%.2fGiB", float64(value)/(1<<30))
}

func micros(value int64) string {
	if value == 0 {
		return "-"
	}
	return offset(value)
}

// offset renders a position on the timeline, where zero means the start --
// which is a place and not an absence, and printing it as "-" said the first
// span had no start.
func offset(value int64) string {
	return (time.Duration(value) * time.Microsecond).String()
}
