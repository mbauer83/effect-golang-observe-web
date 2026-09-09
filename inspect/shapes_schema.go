package inspect

// The descriptions of what the inspector puts on the wire.
//
// Separate from the shapes because they are the whole of one job: a schema per
// type, in the order the types are declared. Together they are what publishes
// the inspector's own contract from the same declarations that serve it -- a
// tool whose API is guessed at is one somebody writes a client for twice.

import (
	"github.com/mbauer83/effect-golang-schema/schema"
)

var eventSchema = schema.Struct[Event]("Event",
	schema.FieldOf("kind", schema.Text(),
		func(value Event) string { return value.Kind },
		func(value *Event, field string) { value.Kind = field }).
		Documented("Kind is the runtime's own event vocabulary."),
	schema.FieldOf("operation", schema.Text(),
		func(value Event) string { return value.Operation },
		func(value *Event, field string) { value.Operation = field }),
	schema.FieldOf("status", schema.Text(),
		func(value Event) string { return value.Status },
		func(value *Event, field string) { value.Status = field }),
	schema.FieldOf("attempt", schema.Int64(),
		func(value Event) int64 { return value.Attempt },
		func(value *Event, field int64) { value.Attempt = field }).
		Documented("Attempt is the retry attempt, or zero where the event is not one."),
	schema.FieldOf("atMicros", schema.Int64(),
		func(value Event) int64 { return value.AtMicros },
		func(value *Event, field int64) { value.AtMicros = field }).
		Documented("AtMicros is the offset from the earliest span in the reading, so an event can be placed on the same timeline the spans are drawn on."),
).Documented("Event is one thing that happened inside a span.")

var spanSchema = schema.Struct[Span]("Span",
	schema.FieldOf("id", schema.Int64(),
		func(value Span) int64 { return value.ID },
		func(value *Span, field int64) { value.ID = field }),
	schema.FieldOf("parentId", schema.Int64(),
		func(value Span) int64 { return value.ParentID },
		func(value *Span, field int64) { value.ParentID = field }),
	schema.FieldOf("trace", schema.Text(),
		func(value Span) string { return value.Trace },
		func(value *Span, field string) { value.Trace = field }).
		Documented("Trace is the identity of the trace this span belongs to, which the runtime's span counter cannot be: it starts again at one in the next process."),
	schema.FieldOf("depth", schema.Int64(),
		func(value Span) int64 { return value.Depth },
		func(value *Span, field int64) { value.Depth = field }).
		Documented("Depth is what nesting carries: the tree flattened, so this shape is not recursive."),
	schema.FieldOf("name", schema.Text(),
		func(value Span) string { return value.Name },
		func(value *Span, field string) { value.Name = field }),
	schema.FieldOf("source", schema.Text(),
		func(value Span) string { return value.Source },
		func(value *Span, field string) { value.Source = field }).
		Documented("Source is the call site the runtime captured when the span was described."),
	schema.FieldOf("status", schema.Text(),
		func(value Span) string { return value.Status },
		func(value *Span, field string) { value.Status = field }),
	schema.FieldOf("startMicros", schema.Int64(),
		func(value Span) int64 { return value.StartMicros },
		func(value *Span, field int64) { value.StartMicros = field }).
		Documented("StartMicros is the offset from the earliest span in this reading: a duration says how long, and only an offset says when."),
	schema.FieldOf("micros", schema.Int64(),
		func(value Span) int64 { return value.Micros },
		func(value *Span, field int64) { value.Micros = field }).
		Documented("Micros is the runtime's own measurement, and zero while the span is open."),
	schema.FieldOf("selfMicros", schema.Int64(),
		func(value Span) int64 { return value.SelfMicros },
		func(value *Span, field int64) { value.SelfMicros = field }).
		Documented("SelfMicros is the duration its children did not take, which is what ranking by hotness ranks by."),
	schema.FieldOf("ageMicros", schema.Int64(),
		func(value Span) int64 { return value.AgeMicros },
		func(value *Span, field int64) { value.AgeMicros = field }).
		Documented("AgeMicros is how long an open span has been open, which is what a duration cannot say."),
	schema.FieldOf("open", schema.Bool(),
		func(value Span) bool { return value.Open },
		func(value *Span, field bool) { value.Open = field }).
		Documented("Open says the span started and has not been seen to end."),
	schema.FieldOf("attributes", schema.List(attributeSchema),
		func(value Span) []Attribute { return value.Attributes },
		func(value *Span, field []Attribute) { value.Attributes = field }),
	schema.FieldOf("events", schema.List(eventSchema),
		func(value Span) []Event { return value.Events },
		func(value *Span, field []Event) { value.Events = field }),
).Documented("Span is one span of a trace, flattened.")

var measurementSchema = schema.Struct[Measurement]("Measurement",
	schema.FieldOf("kind", schema.Text(),
		func(value Measurement) string { return value.Kind },
		func(value *Measurement, field string) { value.Kind = field }),
	schema.FieldOf("operation", schema.Text(),
		func(value Measurement) string { return value.Operation },
		func(value *Measurement, field string) { value.Operation = field }).
		Documented("Operation is a declared name, or the bounded catch-all."),
	schema.FieldOf("status", schema.Text(),
		func(value Measurement) string { return value.Status },
		func(value *Measurement, field string) { value.Status = field }),
	schema.FieldOf("count", schema.Int64(),
		func(value Measurement) int64 { return value.Count },
		func(value *Measurement, field int64) { value.Count = field }),
	schema.FieldOf("medianMicros", schema.Int64(),
		func(value Measurement) int64 { return value.MedianMicros },
		func(value *Measurement, field int64) { value.MedianMicros = field }).
		Documented("MedianMicros is a bucket bound, not an interpolation: the bound at or below which half the measurements fell."),
	schema.FieldOf("maxMicros", schema.Int64(),
		func(value Measurement) int64 { return value.MaxMicros },
		func(value *Measurement, field int64) { value.MaxMicros = field }),
	schema.FieldOf("waitedMicros", schema.Int64(),
		func(value Measurement) int64 { return value.WaitedMicros },
		func(value *Measurement, field int64) { value.WaitedMicros = field }),
).Documented("Measurement is one bounded label's counts and timings.")

var routeSchema = schema.Struct[Route]("Route",
	schema.FieldOf("method", schema.Text(),
		func(value Route) string { return value.Method },
		func(value *Route, field string) { value.Method = field }),
	schema.FieldOf("path", schema.Text(),
		func(value Route) string { return value.Path },
		func(value *Route, field string) { value.Path = field }),
	schema.FieldOf("summary", schema.Text(),
		func(value Route) string { return value.Summary },
		func(value *Route, field string) { value.Summary = field }),
	schema.FieldOf("status", schema.Int64(),
		func(value Route) int64 { return value.Status },
		func(value *Route, field int64) { value.Status = field }).
		Documented("Status is what the endpoint answers with when it succeeds."),
).Documented("Route is one endpoint of the surface being served.")

// SnapshotSchema describes what GET /snapshot answers with.
var SnapshotSchema = schema.Struct[Snapshot]("Snapshot",
	// A formatted string and not a time.Time: the page renders it and does no
	// arithmetic on it, and the format is what an OpenAPI reader wants to see.
	schema.FieldOf("takenAt", schema.Formatted("date-time"),
		func(value Snapshot) string { return value.TakenAt },
		func(value *Snapshot, field string) { value.TakenAt = field }),
	schema.FieldOf("process", processSchema,
		func(value Snapshot) Process { return value.Process },
		func(value *Snapshot, field Process) { value.Process = field }),
	schema.FieldOf("costs", schema.List(costSchema),
		func(value Snapshot) []Cost { return value.Costs },
		func(value *Snapshot, field []Cost) { value.Costs = field }).
		Documented("Costs are what the process spent while each route's work ran, the most allocated first."),
	schema.FieldOf("fibers", schema.List(fiberSchema),
		func(value Snapshot) []Fiber { return value.Fibers },
		func(value *Snapshot, field []Fiber) { value.Fibers = field }).
		Documented("Fibers are the fibers running at this instant, nested as they were forked and flattened by depth."),
	schema.FieldOf("owned", ownedSchema,
		func(value Snapshot) Owned { return value.Owned },
		func(value *Snapshot, field Owned) { value.Owned = field }),
	schema.FieldOf("openSpans", schema.List(spanSchema),
		func(value Snapshot) []Span { return value.OpenSpans },
		func(value *Snapshot, field []Span) { value.OpenSpans = field }).
		Documented("OpenSpans are the spans running at this instant."),
	schema.FieldOf("trace", schema.List(spanSchema),
		func(value Snapshot) []Span { return value.Trace },
		func(value *Snapshot, field []Span) { value.Trace = field }).
		Documented("Trace is the recent events read as a tree, flattened."),
	schema.FieldOf("looseEvents", schema.Int64(),
		func(value Snapshot) int64 { return value.LooseEvents },
		func(value *Snapshot, field int64) { value.LooseEvents = field }).
		Documented("LooseEvents belong to no span in the window: work outside every span, or a span that started before it."),
	schema.FieldOf("measurements", schema.List(measurementSchema),
		func(value Snapshot) []Measurement { return value.Measurements },
		func(value *Snapshot, field []Measurement) { value.Measurements = field }),
	schema.FieldOf("routes", schema.List(routeSchema),
		func(value Snapshot) []Route { return value.Routes },
		func(value *Snapshot, field []Route) { value.Routes = field }),
	schema.FieldOf("dropped", schema.Int64(),
		func(value Snapshot) int64 { return value.Dropped },
		func(value *Snapshot, field int64) { value.Dropped = field }).
		Documented("Dropped is how many events a queued observer discarded, which a tool reporting telemetry should report about itself."),
).Documented("Snapshot is one reading of a program's own telemetry.")

var attributeSchema = schema.Struct[Attribute]("Attribute",
	schema.FieldOf("key", schema.Text(),
		func(value Attribute) string { return value.Key },
		func(value *Attribute, field string) { value.Key = field }),
	schema.FieldOf("value", schema.Text(),
		func(value Attribute) string { return value.Value },
		func(value *Attribute, field string) { value.Value = field }),
).Documented("Attribute is one of a span's annotations, rendered.")

var fiberSchema = schema.Struct[Fiber]("Fiber",
	schema.FieldOf("id", schema.Int64(),
		func(value Fiber) int64 { return value.ID },
		func(value *Fiber, field int64) { value.ID = field }),
	schema.FieldOf("parentId", schema.Int64(),
		func(value Fiber) int64 { return value.ParentID },
		func(value *Fiber, field int64) { value.ParentID = field }),
	schema.FieldOf("depth", schema.Int64(),
		func(value Fiber) int64 { return value.Depth },
		func(value *Fiber, field int64) { value.Depth = field }),
	schema.FieldOf("operation", schema.Text(),
		func(value Fiber) string { return value.Operation },
		func(value *Fiber, field string) { value.Operation = field }).
		Documented("Operation is what was running when the fiber was forked, which is the nearest thing to a name a fiber has."),
	schema.FieldOf("spanId", schema.Int64(),
		func(value Fiber) int64 { return value.SpanID },
		func(value *Fiber, field int64) { value.SpanID = field }),
	schema.FieldOf("ageMicros", schema.Int64(),
		func(value Fiber) int64 { return value.AgeMicros },
		func(value *Fiber, field int64) { value.AgeMicros = field }).
		Documented("AgeMicros is how long it has been running, which is the first thing to know about a fiber that is still going."),
).Documented("Fiber is one fiber the runtime is running.")

var ownedSchema = schema.Struct[Owned]("Owned",
	schema.FieldOf("counted", schema.Bool(),
		func(value Owned) bool { return value.Counted },
		func(value *Owned, field bool) { value.Counted = field }).
		Documented("Counted says the runtime was built to keep these counters; without it the two numbers are zero because nobody is counting."),
	schema.FieldOf("fibers", schema.Int64(),
		func(value Owned) int64 { return value.Fibers },
		func(value *Owned, field int64) { value.Fibers = field }),
	schema.FieldOf("resources", schema.Int64(),
		func(value Owned) int64 { return value.Resources },
		func(value *Owned, field int64) { value.Resources = field }),
).Documented("Owned is what the runtime still holds.")
