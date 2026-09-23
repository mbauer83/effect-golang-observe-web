package inspect

// Reading a program's telemetry into the shapes the wire carries.

import (
	"time"

	"github.com/mbauer83/effect-golang-schema/schema"

	"github.com/mbauer83/effect-golang-observe/metrics"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Telemetry is what an inspector reads.
//
// Named parts rather than an interface, because there is nothing to abstract
// over: these are the values observe produces and the declarations a
// web.Routes already carries. Assemble them with NewTelemetry, or by hand when a
// program wants an arrangement NewTelemetry does not offer.
//
// A handle and not a value: every field in it is already a live pointer to
// something a running program is writing to, so it is passed and held as
// *Telemetry throughout. That is not a style preference -- when it was passed
// by value, assigning a field after handing it to Routes assigned it to a
// copy the inspector had already taken, which is a mistake that compiles,
// runs, and shows an empty panel.
//
// Every field is optional. An inspector with no collector shows no
// measurements rather than refusing to start, because a program that only
// wants to see its spans should not have to aggregate to get them.
type Telemetry struct {
	// Spans is the live span tracker: what is open at this instant.
	Spans *trace.Spans
	// Window is the recent events, folded into a trace on each reading.
	Window *observe.Recent
	// Collector is the bounded aggregate.
	Collector *metrics.Collector
	// Fibers is the live fiber tracker: the execution structure beside the
	// logical one, and the one a program that has stopped responding is found
	// through.
	Fibers *trace.Fibers
	// Buffer is the buffering observer, read only for how much it discarded.
	Buffer *observe.Buffer
	// LiveWork counts what the runtime still holds. A method rather than the
	// runtime itself, so an inspector can be given the counts without being
	// given the thing that can close the program it is watching.
	LiveWork func() effect.LiveWork
	// Series is the process's memory and compute over time. A reading is
	// taken when a snapshot is, so the resolution of a chart is the rate the
	// inspector is being looked at -- and a program nobody is watching pays
	// nothing.
	Series *process.Series
	// Costs is what the process spent while each route's work ran, which
	// inspect.Tracer records.
	Costs *process.Costs
	// Surface names the routes being served, which is what a runtime-level
	// tool cannot know.
	//
	// Assigned whenever it is known, which for a surface that includes the
	// inspector's own routes is after those routes have been assembled. That
	// works because this is a handle: the inspector reads the field when a
	// snapshot is taken, not when it was given the handle.
	Surface []web.Declaration
}

// Take reads everything at once.
//
// Not at one instant: the live spans, the window and the aggregate are three
// values read one after another, and a program does not stop between them. A
// snapshot is therefore nearly consistent rather than consistent, which is
// what any tool reading a running program gets and is worth saying rather
// than implying.
func (telemetry *Telemetry) Take(now time.Time) Snapshot {
	snapshot := Snapshot{
		Time:         now.UTC().Format(time.RFC3339Nano),
		Fibers:       []Fiber{},
		OpenSpans:    []Span{},
		Trace:        []Span{},
		Measurements: []Measurement{},
		Routes:       []Route{},
		Costs:        []Cost{},
	}
	if len(telemetry.Surface) > 0 {
		snapshot.Routes = routesOf(telemetry.Surface)
	}
	if telemetry.Spans != nil {
		snapshot.OpenSpans = flatten(telemetry.Spans.Open(), now)
	}
	if telemetry.Fibers != nil {
		snapshot.Fibers = flattenFibers(telemetry.Fibers.Tree(), now)
	}
	if telemetry.LiveWork != nil {
		work := telemetry.LiveWork()
		snapshot.LiveWork = LiveWork{
			Counted:   true,
			Fibers:    int64(work.Fibers),
			Resources: int64(work.Resources),
		}
	}
	if telemetry.Series != nil {
		snapshot.Process = sampleProcess(telemetry.Series)
	}
	// The window before the accounts, because both are placed on one timeline
	// and the spans decide where it starts: a run's window is only
	// attributable to a span if the two are measured from the same origin.
	origin := time.Time{}
	if telemetry.Window != nil {
		tree := trace.Assemble(telemetry.Window.Events())
		origin = earliest(tree.Spans())
		snapshot.Trace = flattenTree(tree, now, origin)
		snapshot.LooseEvents = int64(len(tree.Loose))
	}
	if telemetry.Costs != nil {
		snapshot.Costs = costsOf(telemetry.Costs.Snapshot(), origin)
	}
	if telemetry.Collector != nil {
		snapshot.Measurements = measurementsOf(telemetry.Collector.Snapshot())
	}
	if telemetry.Buffer != nil {
		snapshot.Drops = int64(telemetry.Buffer.Drops())
	}
	return snapshot
}

// Read decodes a snapshot through the description it was written with.
//
// Public because the inspector's own client should be a client of a contract
// rather than of a guess: a script polling the snapshot, another tool
// aggregating several programs, or a test asserting on what a page will
// render. The description is published at the inspector's own OpenAPI
// endpoint, so the two cannot drift.
func Read(entity []byte) (Snapshot, error) {
	snapshot, err := schema.DecodeJSON(SnapshotSchema, entity)
	if err != nil {
		return Snapshot{}, Fault{Op: "reading a snapshot", Err: err}
	}
	return snapshot, nil
}

// Write encodes a snapshot through the description it is published under.
//
// The counterpart of Read, and what the snapshot endpoint hands over. It is
// public for the same reason: a test that asserts on what a page will render
// should encode what the inspector encodes, not something that resembles it.
func Write(snapshot Snapshot) ([]byte, error) {
	entity, err := schema.EncodeJSON(SnapshotSchema, snapshot)
	if err != nil {
		return nil, Fault{Op: "writing a snapshot", Err: err}
	}
	return entity, nil
}
