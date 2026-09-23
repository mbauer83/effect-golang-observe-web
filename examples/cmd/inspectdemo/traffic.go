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
	for _, one := range traffic() {
		if _, err := send(client, one); err != nil {
			fmt.Printf("  %s: %v\n", one.what, err)
		}
	}
}

type request struct {
	what   string
	method string
	path   string
	body   []byte
}

func traffic() []request {
	return []request{
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

func send(client *web.Client, one request) (web.ClientResponse, error) {
	options := web.ClientRequest{Entity: one.body}
	if len(one.body) > 0 {
		options.MediaType = "application/json"
	}
	exit := effect.Run(context.Background(), effect.Unit{},
		web.Fetch[effect.Unit](client, one.method, one.path, options))
	response, ok := exit.Value()
	if !ok {
		cause, _ := exit.Cause()
		return web.ClientResponse{}, errors.New(cause.String())
	}
	return response, nil
}

// report reads the inspector's own snapshot, through the client, and prints
// what it says -- which is the same reading the page renders.
//
// Twice, with a pause: a reading is taken when a snapshot is, and a window
// needs two of them. One reading gives every gauge and no rate, which is what
// the page shows on its first refresh too.
func report(base string) {
	client := web.Dial(http.DefaultClient, base)
	if _, err := send(client, request{
		method: http.MethodGet, path: inspect.DefaultAt + "/snapshot",
	}); err != nil {
		fail(err)
	}
	time.Sleep(300 * time.Millisecond)
	response, err := send(client, request{
		method: http.MethodGet, path: inspect.DefaultAt + "/snapshot",
	})
	if err != nil {
		fail(err)
	}
	snapshot, err := inspect.Read(response.Entity)
	if err != nil {
		fail(err)
	}

	fmt.Printf("\nthe inspector, read at %s\n", snapshot.Time)
	fmt.Printf("  the runtime owns %d fiber(s) and %d resource(s)\n",
		snapshot.LiveWork.Fibers, snapshot.LiveWork.Resources)
	fmt.Printf("  %d span(s) in the window, %d event(s) outside every span\n",
		len(snapshot.Trace), snapshot.LooseEvents)
	for _, span := range snapshot.Trace {
		fmt.Printf("  %s%-28s +%-10s %-10s %s\n",
			indent(span.Depth), span.Name,
			offset(span.StartMicros), micros(span.Micros), span.Status)
	}
	fmt.Println("\n  what was served")
	for _, route := range snapshot.Routes {
		fmt.Printf("    %-6s %-18s answers %d  %s\n",
			route.Method, route.Path, route.Status, route.Summary)
	}
	fmt.Printf("\n  the process, %s into the window\n",
		micros(snapshot.Process.OverMicros))
	fmt.Printf("    heap %s of a %s goal, %d objects, stacks %s\n",
		bytes(snapshot.Process.HeapBytes), bytes(snapshot.Process.GoalBytes),
		snapshot.Process.HeapObjects, bytes(snapshot.Process.StackBytes))
	fmt.Printf("    %d goroutine(s) on %d thread(s): %d running, %d runnable, %d waiting\n",
		snapshot.Process.Goroutines, snapshot.Process.Threads,
		snapshot.Process.Running, snapshot.Process.Runnable, snapshot.Process.Waiting)
	fmt.Printf("    allocated %s at %s/s, %.1f%% busy, %.1f%% of that collecting\n",
		bytes(snapshot.Process.AllocBytes), bytes(int64(snapshot.Process.BytesPerSecond)),
		100*snapshot.Process.Busy, 100*snapshot.Process.GCShare)

	if len(snapshot.Costs) > 0 {
		fmt.Println("\n  what the process spent while each route ran")
		fmt.Println("    (process-wide over each request's window, so concurrent work is in it too)")
		for _, cost := range snapshot.Costs {
			fmt.Printf("    %-24s x%-3d %8s per run, longest %s\n",
				cost.Name, cost.Times, bytes(cost.PerRunBytes),
				micros(cost.LongestMicros))
		}
	}

	fmt.Println("\n  what it measured, per route")
	for _, measurement := range snapshot.Measurements {
		if measurement.Kind != "span_ended" {
			continue
		}
		fmt.Printf("    %-24s %-14s x%d  median at most %s\n",
			measurement.Operation, measurement.Status, measurement.Count,
			micros(measurement.MedianMicros))
	}
}

func indent(depth int64) string {
	result := ""
	for range depth {
		result += "  "
	}
	return result
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
