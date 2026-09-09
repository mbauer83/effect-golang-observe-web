package acceptance

// Which trace a span belongs to, and which run a span was.
//
// A different claim from what a request decomposes into: that one is about the
// shape of a trace, and these two are about naming it and attributing a
// measurement to one span of it. Both are what the timeline's details are
// built on, and both are joins that would fail silently -- a page showing the
// wrong numbers rather than none.

import (
	"net/http"
	"testing"

	"github.com/mbauer83/effect-golang-observe-web/inspect"
)

func TestEverySpanOfATraceCarriesTheSameIdentityAndTracesDiffer(t *testing.T) {
	// The runtime's span identity is a counter, so a page keying a selection
	// on it would key on a different trace in the next process. What a
	// selection can be kept by is this.
	client := inspecting(t)
	fetched(t, client, http.MethodGet, "/notes/report")
	fetched(t, client, http.MethodGet, "/notes")

	read := taken(t, client)
	perRoot := map[int64]string{}
	rootOf := map[int64]int64{}
	for _, span := range read.Trace {
		root := span.ID
		if span.ParentID != 0 {
			root = rootOf[span.ParentID]
		}
		rootOf[span.ID] = root
		if span.Trace == "" {
			t.Fatalf("expected span %d to name its trace", span.ID)
		}
		if held, seen := perRoot[root]; seen && held != span.Trace {
			t.Fatalf("expected one identity per trace, got %q and %q under %d",
				held, span.Trace, root)
		}
		perRoot[root] = span.Trace
	}
	if len(perRoot) < 2 {
		t.Fatalf("expected at least two traces, got %d", len(perRoot))
	}
	distinct := map[string]bool{}
	for _, identity := range perRoot {
		distinct[identity] = true
	}
	if len(distinct) != len(perRoot) {
		t.Fatalf("expected %d identities, got %d", len(perRoot), len(distinct))
	}
}

func TestAStagesRunIsAttributableToItsSpan(t *testing.T) {
	// The join the timeline's details are built on: a name's account is an
	// average over every run of it, and the run whose measured window closed
	// inside a span is what *that* span did. Both sides are offset from the
	// same origin, which is what makes the comparison meaningful at all.
	client := inspecting(t)
	fetched(t, client, http.MethodGet, "/notes/report")

	read := taken(t, client)
	runs := map[string][]inspect.Run{}
	for _, cost := range read.Costs {
		runs[cost.Name] = cost.Runs
	}
	for _, span := range read.Trace {
		held, accounted := runs[span.Name]
		if !accounted {
			continue
		}
		within := 0
		for _, run := range held {
			if run.EndedMicros >= span.StartMicros-1 &&
				run.EndedMicros <= span.StartMicros+span.Micros+1 {
				within++
			}
		}
		if within != 1 {
			t.Fatalf("expected one run of %q inside its span (%d..%d), got %d of %v",
				span.Name, span.StartMicros, span.StartMicros+span.Micros,
				within, held)
		}
	}
}
