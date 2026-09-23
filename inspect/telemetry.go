package inspect

// Assembling the whole arrangement in one call.
//
// What every program watching itself needs, and what every program watching
// itself was writing out: two live trackers, a window, a bounded aggregate, a
// process series, a cost account, a queue in front of the two that take locks,
// and a fanout over the lot. Forty lines that are the same forty lines every
// time, with one detail -- which observers go behind the queue and which stay
// inline -- that is not obvious and is wrong quietly when it is wrong.
//
// So it is here, once, with the reasoning attached. A program that wants an
// arrangement this does not offer still assembles a Telemetry by hand; nothing
// about that got harder.

import (
	"github.com/mbauer83/effect-golang-observe/metrics"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-observe/trace"
	"github.com/mbauer83/effect-golang/effect"
)

// TelemetryConfig are what a program wants watched, and how much of it to keep.
//
// Every length has a default that is a sensible answer rather than a minimum,
// so a caller states the ones it has an opinion about. The defaults are stated
// as constants below with what each length means in practice, because "4096"
// tells a reader nothing and "a few hundred requests' worth" tells them
// whether it is enough.
type TelemetryConfig struct {
	// Names is the metric vocabulary: the operation names worth their own
	// measurements. Everything else is counted under one name, so a name left
	// out is traffic in a bucket called "other".
	//
	// Names(surface.Declarations()) is where a web program's come from, plus
	// web.PhaseNames() for the phases each request is broken into, plus
	// Names of the inspector's own routes -- because an inspector being
	// looked at hard is a load worth seeing as its own.
	Names []string
	// Recent is how many events the span tree is assembled from. Long enough
	// to hold a busy request's whole descendancy, short enough that reading it
	// is cheap.
	Recent int
	// ChartPoints is how many memory and compute readings a chart keeps. A length
	// in refreshes and not in seconds, because a reading is taken when a
	// snapshot is -- so a program nobody is looking at pays nothing and keeps
	// nothing.
	ChartPoints int
	// QueueCapacity is how many events may wait to be counted.
	QueueCapacity int
	// Detail is how finely each name's allocation is broken down.
	Detail Detail
}

// Detail is how much is kept about what each name allocated.
//
// Two named cases rather than a flag, because the choice is a real one with a
// cost on each side and "true" is not a word anybody can read: classes keep a
// set of size buckets per name, which is what tells a leak of many small
// objects from a leak of few large ones, and scalars keep two numbers.
type Detail string

const (
	// Scalars is what each name allocated and how many objects, which is
	// enough to find the expensive route.
	Scalars Detail = "scalars"
	// Classes is that, plus the size classes it allocated in -- which is what
	// answers "many small or few large" about the expensive one.
	Classes Detail = "classes"
)

// Defaults for the lengths, as amounts rather than numbers.
const (
	// defaultRecent is a few hundred requests' worth of events on a surface
	// that opens a handful of spans per request.
	defaultRecent = 4096
	// defaultChartPoints is a few minutes of chart at a couple of seconds a
	// refresh.
	defaultChartPoints = 180
	// defaultQueueCapacity is enough that a burst of traffic is counted rather
	// than sampled, and bounded so that a program nobody drains cannot grow.
	defaultQueueCapacity = 8192
)

// NewTelemetry is the standard arrangement: what to read, and the observer that
// feeds it.
//
// Two returns because they go to two places -- the handle to the routes, the
// observer to effect.NewRuntime -- and a caller that got one thing holding
// both would be asking the inspector for the means to close the program it is
// inspecting.
//
//	telemetry, observer, err := inspect.NewTelemetry(inspect.TelemetryConfig{Names: names})
//	runtime, err := effect.NewRuntime(effect.WithObserver(observing))
//	telemetry.LiveWork = runtime.LiveWork
//
// The observer is not optional and not nil: a program that does not want to be
// watched does not call this, and gets a runtime with no observer at all --
// which is cheaper than one with an observer that discards, because the
// runtime checks whether it is observed before it builds an event.
func NewTelemetry(terms TelemetryConfig) (*Telemetry, effect.Observer, error) {
	window, err := observe.NewRecent(orDefault(terms.Recent, defaultRecent))
	if err != nil {
		return nil, nil, err
	}
	series, err := process.NewSeries(orDefault(terms.ChartPoints, defaultChartPoints))
	if err != nil {
		return nil, nil, err
	}
	telemetry := &Telemetry{
		Spans:     trace.NewSpans(),
		Fibers:    trace.NewFibers(),
		Window:    window,
		Collector: metrics.NewCollector(metrics.NewVocabulary(terms.Names...)),
		Series:    series,
		Costs:     accountFor(terms.Detail, terms.Names),
	}
	// The aggregate and the window go behind the queue; the live trackers stay
	// in front of it. That division is the one detail in this arrangement that
	// is not obvious: the first two take a lock and hold it while they fold an
	// event in, and a request should never wait for either -- an observer that
	// made every span queue for a mutex would change what it was measuring.
	// The live trackers are what a program that has stopped responding is
	// found through, so they must not be behind a queue nothing is draining.
	buffer, err := observe.NewBuffer(
		observe.Fanout(telemetry.Collector, telemetry.Window),
		orDefault(terms.QueueCapacity, defaultQueueCapacity),
		// Oldest, because this is a window on the recent past: a full queue
		// means a burst, and during a burst the events worth keeping are the
		// ones that just happened.
		observe.DropOldest,
	)
	if err != nil {
		return nil, nil, err
	}
	telemetry.Buffer = buffer
	return telemetry, observe.Fanout(telemetry.Spans, telemetry.Fibers, buffer), nil
}

// accountFor is the cost account this detail asks for.
func accountFor(detail Detail, names []string) *process.Costs {
	if detail == Classes {
		return process.NewCostsWithSizes(names...)
	}
	return process.NewCosts(names...)
}

// orDefault is a stated length, or the default when nothing was stated.
//
// A zero is "no opinion" and not "keep nothing", because keeping nothing is
// what leaving the whole call out already says.
func orDefault(length int, fallback int) int {
	if length > 0 {
		return length
	}
	return fallback
}
