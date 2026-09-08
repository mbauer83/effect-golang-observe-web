package inspect

// Reading observe's and web's values into the shapes the wire carries.
//
// A tree becomes a list with a depth, a duration becomes microseconds, and an
// annotation becomes two strings. Each conversion is here rather than beside
// the shape it produces, because together they are one job: everything the
// page needs and nothing it does not.

import (
	"time"

	"github.com/mbauer83/effect-golang-observe/metrics"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang-web/web"
)

// flattenTree walks the tree in reading order, records the depth that the
// recursion carried, and offsets every span from the earliest of them.
//
// The offsets are relative because that is all a page needs and all it should
// be given: an absolute clock reading would make the page's arithmetic depend
// on whose clock it was.
func flattenTree(assembled trace.Trace, now time.Time) []Span {
	spans := assembled.Spans()
	origin := earliest(spans)
	flattened := []Span{}
	assembled.Walk(func(span trace.Span, depth int) {
		flattened = append(flattened, spanOf(span, int64(depth), now, origin))
	})
	return flattened
}

// earliest is the start of the earliest span, or the zero time when there are
// none.
func earliest(spans []trace.Span) time.Time {
	origin := time.Time{}
	for _, span := range spans {
		if origin.IsZero() || span.Started.Before(origin) {
			origin = span.Started
		}
	}
	return origin
}

// flatten is the same for a list of spans that has no tree: the live ones,
// each at depth zero because what encloses them may not be in the list.
func flatten(spans []trace.Span, now time.Time) []Span {
	origin := earliest(spans)
	flattened := make([]Span, 0, len(spans))
	for _, span := range spans {
		flattened = append(flattened, spanOf(span, 0, now, origin))
	}
	return flattened
}

// flattenFibers walks the forked tree and records the depth, as the spans do.
func flattenFibers(fibers []trace.Fiber, now time.Time) []Fiber {
	flattened := []Fiber{}
	for _, fiber := range fibers {
		flattened = appendFiber(flattened, fiber, 0, now)
	}
	return flattened
}

func appendFiber(into []Fiber, fiber trace.Fiber, depth int64, now time.Time) []Fiber {
	into = append(into, Fiber{
		ID:        int64(fiber.ID),
		ParentID:  int64(fiber.ParentID),
		Depth:     depth,
		Operation: fiber.Operation,
		SpanID:    int64(fiber.SpanID),
		AgeMicros: fiber.Age(now).Microseconds(),
	})
	for _, child := range fiber.Children {
		into = appendFiber(into, child, depth+1, now)
	}
	return into
}

// spanOf reads one span, with every offset measured from the same origin so
// the spans and their events share one timeline.
func spanOf(span trace.Span, depth int64, now time.Time, origin time.Time) Span {
	events := make([]Event, 0, len(span.Events))
	for _, event := range span.Events {
		events = append(events, Event{
			Kind:      string(event.Kind),
			Operation: event.Operation,
			Status:    string(event.Status),
			Attempt:   int64(event.Attempt),
			AtMicros:  offsetFrom(origin, event.Timestamp),
		})
	}
	attributes := make([]Attribute, 0, len(span.Attributes))
	for _, held := range span.Attributes {
		attributes = append(attributes,
			Attribute{Key: held.Key, Value: held.Value.String()})
	}
	written := Span{
		StartMicros: offsetFrom(origin, span.Started),
		ID:          int64(span.ID),
		ParentID:    int64(span.ParentID),
		Depth:       depth,
		Name:        span.Name,
		Source:      span.Source,
		Status:      string(span.Status),
		Micros:      span.Duration.Microseconds(),
		SelfMicros:  span.Self().Microseconds(),
		Open:        span.Open(),
		Attributes:  attributes,
		Events:      events,
	}
	if span.Open() {
		written.AgeMicros = span.Age(now).Microseconds()
	}
	return written
}

func measurementsOf(snapshot metrics.Snapshot) []Measurement {
	measured := make([]Measurement, 0, len(snapshot.Counts))
	for _, label := range snapshot.Labels() {
		one := Measurement{
			Kind:      string(label.Kind),
			Operation: label.Operation,
			Status:    string(label.Status),
			Count:     int64(snapshot.Counts[label]),
		}
		if held, timed := snapshot.Durations[label]; timed {
			one.MedianMicros = held.Quantile(0.5).Microseconds()
			one.MaxMicros = held.Max.Microseconds()
		}
		if held, waited := snapshot.Delays[label]; waited {
			one.WaitedMicros = held.Sum.Microseconds()
		}
		measured = append(measured, one)
	}
	return measured
}

func routesOf(declarations []web.Declaration) []Route {
	described := make([]Route, 0, len(declarations))
	for _, declaration := range declarations {
		described = append(described, Route{
			Method:  declaration.Method,
			Path:    declaration.Path,
			Summary: declaration.Summary,
			Status:  int64(declaration.Status),
		})
	}
	return described
}

// offsetFrom is a moment as microseconds after the origin, and zero where
// there is no origin to measure from or the moment predates it.
func offsetFrom(origin time.Time, at time.Time) int64 {
	if origin.IsZero() || at.Before(origin) {
		return 0
	}
	return at.Sub(origin).Microseconds()
}
