package unit

// Reading observe's and web's values into the wire shapes, which is where the
// decisions a page depends on are made: the tree flattened by depth, the
// offsets relative to the reading, and a zero that means the start rather than
// nothing.

import (
	"log/slog"
	"testing"
	"time"

	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

var origin = time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)

func at(offset time.Duration) time.Time { return origin.Add(offset) }

func spanStarted(id uint64, parent uint64, name string, offset time.Duration) effect.RuntimeEvent {
	return effect.RuntimeEvent{
		Kind:      effect.EventSpanStarted,
		Timestamp: at(offset),
		Operation: name,
		Source:    name + ".go:1",
		SpanID:    id,
		ParentID:  parent,
	}
}

func spanEnded(
	id uint64,
	name string,
	offset time.Duration,
	took time.Duration,
	status effect.EventStatus,
) effect.RuntimeEvent {
	return effect.RuntimeEvent{
		Kind:       effect.EventSpanEnded,
		Timestamp:  at(offset),
		Duration:   took,
		Operation:  name,
		SpanID:     id,
		Status:     status,
		Attributes: []slog.Attr{slog.String("route", "/notes")},
	}
}

// windowOf is a window holding the events given, which is what the inspector
// folds a trace from.
func windowOf(t *testing.T, events ...effect.RuntimeEvent) *observe.Recent {
	t.Helper()
	window, err := observe.Keep(64)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		window.Observe(nil, event)
	}
	return window
}

func TestTheTreeIsFlattenedByDepthAndOffsetFromTheEarliestSpan(t *testing.T) {
	// Both decisions a page depends on. The depth carries what the recursion
	// carried, so neither the description nor the page has to recurse; the
	// offsets are relative, because an absolute clock reading would make the
	// page's arithmetic depend on whose clock it was.
	watched := &inspect.Watched{Window: windowOf(t,
		spanStarted(1, 0, "outer", 0),
		spanStarted(2, 1, "inner", 2*time.Millisecond),
		spanEnded(2, "inner", 3*time.Millisecond, time.Millisecond, effect.EventStatusSuccess),
		spanEnded(1, "outer", 5*time.Millisecond, 5*time.Millisecond, effect.EventStatusSuccess),
	)}

	taken := watched.Take(at(time.Second))
	if len(taken.Trace) != 2 {
		t.Fatalf("expected both spans, got %d", len(taken.Trace))
	}
	outer, inner := taken.Trace[0], taken.Trace[1]
	if outer.Depth != 0 || inner.Depth != 1 {
		t.Fatalf("expected the nesting as depths, got %d and %d", outer.Depth, inner.Depth)
	}
	// The earliest span is the origin, so its offset is zero -- a place on the
	// timeline and not an absence.
	if outer.StartMicros != 0 {
		t.Fatalf("expected the earliest span at the origin, got %d", outer.StartMicros)
	}
	if inner.StartMicros != 2000 {
		t.Fatalf("expected the inner span offset by 2ms, got %dµs", inner.StartMicros)
	}
	if outer.Micros != 5000 || inner.Micros != 1000 {
		t.Fatalf("unexpected durations: %d and %d", outer.Micros, inner.Micros)
	}
	// The call site and the annotations come through, because a span with
	// neither is a span a reader cannot place.
	if outer.Source != "outer.go:1" {
		t.Fatalf("expected the call site, got %q", outer.Source)
	}
	if len(outer.Attributes) != 1 || outer.Attributes[0].Key != "route" {
		t.Fatalf("expected the annotation, got %v", outer.Attributes)
	}
}

func TestAnOpenSpanCarriesItsAgeBecauseItHasNoDuration(t *testing.T) {
	running := trace.Watch()
	running.Observe(nil, spanStarted(1, 0, "waiting", 0))
	watched := &inspect.Watched{Running: running}

	taken := watched.Take(at(2 * time.Minute))
	if len(taken.OpenSpans) != 1 {
		t.Fatalf("expected the open span, got %d", len(taken.OpenSpans))
	}
	open := taken.OpenSpans[0]
	if !open.Open || open.Micros != 0 {
		t.Fatalf("expected an open span with no duration, got %+v", open)
	}
	if open.AgeMicros != (2 * time.Minute).Microseconds() {
		t.Fatalf("expected the age since it started, got %d", open.AgeMicros)
	}
}

func TestRunningFibersAreFlattenedWithTheirAges(t *testing.T) {
	fibers := trace.WatchFibers()
	fibers.Observe(nil, effect.RuntimeEvent{
		Kind: effect.EventFiberStarted, Timestamp: at(0),
		Operation: "serve", FiberID: 1,
	})
	fibers.Observe(nil, effect.RuntimeEvent{
		Kind: effect.EventFiberStarted, Timestamp: at(time.Second),
		Operation: "handle", FiberID: 2, ParentFiber: 1,
	})
	watched := &inspect.Watched{Fibers: fibers}

	taken := watched.Take(at(3 * time.Second))
	if len(taken.Fibers) != 2 {
		t.Fatalf("expected both fibers, got %d", len(taken.Fibers))
	}
	if taken.Fibers[0].Depth != 0 || taken.Fibers[1].Depth != 1 {
		t.Fatalf("expected the forking as depths, got %v", taken.Fibers)
	}
	if taken.Fibers[0].AgeMicros != (3 * time.Second).Microseconds() {
		t.Fatalf("expected the forker's age, got %d", taken.Fibers[0].AgeMicros)
	}
}

func TestARuntimeThatIsNotCountingSaysSoRatherThanReportingZero(t *testing.T) {
	// Two zeroes and "nobody is counting" look identical otherwise, and the
	// difference matters: the counters are off by default because they cost a
	// pair of atomics per fiber and per resource.
	if taken := (&inspect.Watched{}).Take(at(0)); taken.Owned.Counted {
		t.Fatalf("expected no count to be claimed, got %+v", taken.Owned)
	}
	watched := &inspect.Watched{
		Owned: func() effect.LiveWork { return effect.LiveWork{Fibers: 2, Resources: 3} },
	}
	taken := watched.Take(at(0))
	if !taken.Owned.Counted || taken.Owned.Fibers != 2 || taken.Owned.Resources != 3 {
		t.Fatalf("unexpected count: %+v", taken.Owned)
	}
}

func TestASnapshotRoundTripsThroughItsOwnDescription(t *testing.T) {
	// What makes a script reading the inspector a client of a contract rather
	// than of a guess: the shape it decodes with is the shape the inspector
	// publishes.
	watched := &inspect.Watched{
		Window: windowOf(t,
			spanStarted(1, 0, "outer", 0),
			spanEnded(1, "outer", time.Millisecond, time.Millisecond, effect.EventStatusSuccess),
		),
		Surface: []web.Declaration{
			{Method: "GET", Path: "/notes", Summary: "List the notes", Status: 200},
		},
	}
	taken := watched.Take(at(0))

	written, err := inspect.Write(taken)
	if err != nil {
		t.Fatal(err)
	}
	read, err := inspect.Read(written)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Trace) != 1 || read.Trace[0].Name != "outer" {
		t.Fatalf("the crossing changed the trace: %#v", read.Trace)
	}
	if len(read.Routes) != 1 || read.Routes[0].Path != "/notes" {
		t.Fatalf("the crossing changed the surface: %#v", read.Routes)
	}
	if read.TakenAt != taken.TakenAt {
		t.Fatalf("the crossing changed the instant: %q against %q", read.TakenAt, taken.TakenAt)
	}
}

func TestTheSurfaceReportedIsWhateverTheCallerSays(t *testing.T) {
	// The inspector reports what it is given and makes no judgement about it.
	// A program that wants its own routes shown reports those; one that wants
	// the inspector's shown too reports both -- which it can assign after
	// handing the handle over, because the field is read when a reading is
	// taken and not when the handle was given.
	watched := &inspect.Watched{}
	watched.Surface = []web.Declaration{
		{Method: "GET", Path: "/notes", Status: 200},
		{Method: "GET", Path: "/inspect/snapshot", Status: 200},
	}

	taken := watched.Take(at(0))
	if len(taken.Routes) != 2 {
		t.Fatalf("expected both declarations reported, got %v", taken.Routes)
	}
	if taken.Routes[1].Path != "/inspect/snapshot" {
		t.Fatalf("expected the inspector's own route reported when asked, got %v", taken.Routes)
	}

	// And nothing at all when there is nothing to say, rather than a nil the
	// description would have to allow.
	if bare := (&inspect.Watched{}).Take(at(0)); bare.Routes == nil || len(bare.Routes) != 0 {
		t.Fatalf("expected an empty surface, got %#v", bare.Routes)
	}
}
