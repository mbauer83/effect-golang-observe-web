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

// Watched is what an inspector reads.
//
// Named parts rather than an interface, because there is nothing to abstract
// over: these are the values observe produces and the declarations a
// web.Routes already carries. Assemble them with Watching, or by hand when a
// program wants an arrangement Watching does not offer.
//
// A handle and not a value: every field in it is already a live pointer to
// something a running program is writing to, so it is passed and held as
// *Watched throughout. That is not a style preference -- when it was passed
// by value, assigning a field after handing it to Routes assigned it to a
// copy the inspector had already taken, which is a mistake that compiles,
// runs, and shows an empty panel.
//
// Every field is optional. An inspector with no collector shows no
// measurements rather than refusing to start, because a program that only
// wants to see its spans should not have to aggregate to get them.
type Watched struct {
	// Running is the live span tracker: what is open at this instant.
	Running *trace.Running
	// Window is the recent events, folded into a trace on each reading.
	Window *observe.Recent
	// Collected is the bounded aggregate.
	Collected *metrics.Collector
	// Fibers is the live fiber tracker: the execution structure beside the
	// logical one, and the one a program that has stopped responding is found
	// through.
	Fibers *trace.Fibers
	// Queued is the buffering observer, read only for how much it discarded.
	Queued *observe.Buffered
	// Owned counts what the runtime still holds. A method rather than the
	// runtime itself, so an inspector can be given the counts without being
	// given the thing that can close the program it is watching.
	Owned func() effect.LiveWork
	// Series is the process's memory and compute over time. A reading is
	// taken when a snapshot is, so the resolution of a chart is the rate the
	// inspector is being looked at -- and a program nobody is watching pays
	// nothing.
	Series *process.Series
	// Costs is what the process spent while each route's work ran, which
	// inspect.Accounted records.
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
func (watched *Watched) Take(now time.Time) Snapshot {
	taken := Snapshot{
		TakenAt:      now.UTC().Format(time.RFC3339Nano),
		Fibers:       []Fiber{},
		OpenSpans:    []Span{},
		Trace:        []Span{},
		Measurements: []Measurement{},
		Routes:       []Route{},
		Costs:        []Cost{},
	}
	if len(watched.Surface) > 0 {
		taken.Routes = routesOf(watched.Surface)
	}
	if watched.Running != nil {
		taken.OpenSpans = flatten(watched.Running.Open(), now)
	}
	if watched.Fibers != nil {
		taken.Fibers = flattenFibers(watched.Fibers.Running(), now)
	}
	if watched.Owned != nil {
		held := watched.Owned()
		taken.Owned = Owned{
			Counted:   true,
			Fibers:    int64(held.Fibers),
			Resources: int64(held.Resources),
		}
	}
	if watched.Series != nil {
		taken.Process = sampled(watched.Series)
	}
	// The window before the accounts, because both are placed on one timeline
	// and the spans decide where it starts: a run's window is only
	// attributable to a span if the two are measured from the same origin.
	origin := time.Time{}
	if watched.Window != nil {
		assembled := trace.Assemble(watched.Window.Events())
		origin = earliest(assembled.Spans())
		taken.Trace = flattenTree(assembled, now, origin)
		taken.LooseEvents = int64(len(assembled.Loose))
	}
	if watched.Costs != nil {
		taken.Costs = costsOf(watched.Costs.Snapshot(), origin)
	}
	if watched.Collected != nil {
		taken.Measurements = measurementsOf(watched.Collected.Snapshot())
	}
	if watched.Queued != nil {
		taken.Dropped = int64(watched.Queued.Dropped())
	}
	return taken
}

// Read decodes a snapshot through the description it was written with.
//
// Public because the inspector's own client should be a client of a contract
// rather than of a guess: a script polling the snapshot, another tool
// aggregating several programs, or a test asserting on what a page will
// render. The description is published at the inspector's own OpenAPI
// endpoint, so the two cannot drift.
func Read(entity []byte) (Snapshot, error) {
	taken, err := schema.DecodeJSON(SnapshotSchema, entity)
	if err != nil {
		return Snapshot{}, Fault{Doing: "reading a snapshot", Err: err}
	}
	return taken, nil
}

// Write encodes a snapshot through the description it is published under.
//
// The counterpart of Read, and what the snapshot endpoint hands over. It is
// public for the same reason: a test that asserts on what a page will render
// should encode what the inspector encodes, not something that resembles it.
func Write(taken Snapshot) ([]byte, error) {
	written, err := schema.EncodeJSON(SnapshotSchema, taken)
	if err != nil {
		return nil, Fault{Doing: "writing a snapshot", Err: err}
	}
	return written, nil
}
