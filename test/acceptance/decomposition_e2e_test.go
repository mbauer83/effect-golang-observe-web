package acceptance

// A request that decomposes, and what the inspector can say about it.
//
// The other suites exercise routes that answer from memory in microseconds: a
// trace of one span, with nothing underneath and nothing to rank. This one
// runs the route with four measured stages, which is what the timeline, the
// self times and the cost account are for.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-observe-web/inspect"
)

func TestAMeasuredStageAppearsAsASpanUnderItsRoute(t *testing.T) {
	client := serveExample(t)
	fetch(t, client, http.MethodGet, "/notes/report")

	snapshot := readSnapshot(t, client)
	under := map[string]inspect.Span{}
	root := inspect.Span{}
	for _, span := range snapshot.Trace {
		if span.Name == "GET /notes/report" {
			root = span
			continue
		}
		if span.Depth > 0 {
			under[span.Name] = span
		}
	}
	if root.Name == "" {
		t.Fatalf("expected the route's own span, got %v", names(snapshot.Trace))
	}
	// The surface is detailing its phases, so the route's own parts are
	// spanned too and the stages sit inside the handler one: route ->
	// handler -> stage. That is the shape a waterfall shows, and asserting
	// the relationship rather than a number keeps this readable when another
	// phase is added.
	handler, phased := under["handle"]
	if !phased || handler.Depth != 1 {
		t.Fatalf("expected a handling phase under the route, got %v", names(snapshot.Trace))
	}
	for _, phase := range []string{"decode", "encode"} {
		if span, present := under[phase]; !present || span.Depth != 1 {
			t.Fatalf("expected the %s phase under the route, got %v", phase, names(snapshot.Trace))
		}
	}
	for _, stage := range []string{"read", "count", "digest", "rank"} {
		span, present := under[stage]
		if !present {
			t.Fatalf("expected the %s stage under the route, got %v", stage, names(snapshot.Trace))
		}
		if span.Depth != handler.Depth+1 {
			t.Fatalf("expected %s inside the handling phase, got depth %d against %d",
				stage, span.Depth, handler.Depth)
		}
		// Each stage begins after the route does and ends before it: a stage
		// outside its route's window would mean the offsets share no origin.
		if span.StartMicros < root.StartMicros {
			t.Fatalf("expected %s to begin within the route, got %d against %d",
				stage, span.StartMicros, root.StartMicros)
		}
	}
}

func TestARoutesSelfTimeExcludesTheStagesInsideIt(t *testing.T) {
	// What ranking by hotness ranks by. The route is the slowest span and
	// almost none of the time is its own.
	client := serveExample(t)
	fetch(t, client, http.MethodGet, "/notes/report")

	snapshot := readSnapshot(t, client)
	for _, span := range snapshot.Trace {
		if span.SelfMicros > span.Micros {
			t.Fatalf("%s claims more of its own than it took: %d of %d",
				span.Name, span.SelfMicros, span.Micros)
		}
		if span.Name != "GET /notes/report" {
			continue
		}
		if span.SelfMicros >= span.Micros {
			t.Fatalf("expected the route to have spent most of itself in its stages, got %d of %d",
				span.SelfMicros, span.Micros)
		}
	}
}

func TestTheRetryingStageCarriesItsAttemptsAsEvents(t *testing.T) {
	// The stage refuses twice before it answers, and the attempts are events
	// on that stage's own span rather than a mystery in the route's total.
	client := serveExample(t)
	fetch(t, client, http.MethodGet, "/notes/report")

	for _, span := range readSnapshot(t, client).Trace {
		if span.Name != "rank" {
			continue
		}
		retries := 0
		for _, event := range span.Events {
			if event.Kind == "retry_scheduled" {
				retries++
			}
			// Every event is placed on the same timeline the spans are, which
			// is what lets it be drawn as a mark on its span's bar rather
			// than listed under it. Within the span's own window, because an
			// event outside it could not be drawn on it.
			if event.AtMicros < span.StartMicros {
				t.Fatalf("expected %s within its span, got %d before %d",
					event.Kind, event.AtMicros, span.StartMicros)
			}
			if event.AtMicros > span.StartMicros+span.Micros {
				t.Fatalf("expected %s within its span, got %d after %d",
					event.Kind, event.AtMicros, span.StartMicros+span.Micros)
			}
		}
		if retries != 2 {
			t.Fatalf("expected the two refusals as events, got %d", retries)
		}
		return
	}
	t.Fatal("expected the ranking stage in the trace")
}

func TestEachStageIsAccountedSeparately(t *testing.T) {
	client := serveExample(t)
	fetch(t, client, http.MethodGet, "/notes/report")

	costs := map[string]inspect.Cost{}
	for _, cost := range readSnapshot(t, client).Costs {
		costs[cost.Name] = cost
	}
	for _, stage := range []string{"read", "count", "digest", "rank"} {
		if cost, present := costs[stage]; !present || cost.Times == 0 {
			t.Fatalf("expected %s accounted with a run, got %+v", stage, cost)
		}
	}
	// And the route itself, which is what Accounted adds over the stages'
	// own Measured: without it the stages are accounted and the request is
	// not, and nothing would say so.
	route, present := costs["GET /notes/report"]
	if !present || route.Times == 0 {
		t.Fatalf("expected the route accounted with a run, got %+v", route)
	}
	// The route's window covers its stages, so it allocated at least as much
	// as the heaviest of them.
	if route.BytesDuring < costs["digest"].BytesDuring {
		t.Fatalf("expected the route to account for its stages, got %d against %d",
			route.BytesDuring, costs["digest"].BytesDuring)
	}
}

func TestTheChartLibraryIsServedFromTheInspectorAndNotFromAnywhereElse(t *testing.T) {
	client := serveExample(t)

	script := fetch(t, client, http.MethodGet, inspect.DefaultAt+"/uplot.js")
	if script.Status != http.StatusOK || len(script.Entity) < 10_000 {
		t.Fatalf("expected the library, got %d and %d bytes",
			script.Status, len(script.Entity))
	}
	if kind := script.Header.Get("Content-Type"); !strings.HasPrefix(kind, "text/javascript") {
		t.Fatalf("expected javascript, got %q", kind)
	}
	if style := fetch(t, client, http.MethodGet, inspect.DefaultAt+"/uplot.css"); style.Status != http.StatusOK {
		t.Fatalf("expected the stylesheet, got %d", style.Status)
	}
	// The licence travels with the code, so attribution is served rather than
	// only committed.
	licence := fetch(t, client, http.MethodGet, inspect.DefaultAt+"/uplot.LICENSE")
	if !strings.Contains(string(licence.Entity), "MIT") {
		t.Fatalf("expected the licence, got %q", string(licence.Entity))
	}
}

func names(spans []inspect.Span) []string {
	result := make([]string, 0, len(spans))
	for _, span := range spans {
		result = append(result, span.Name)
	}
	return result
}
