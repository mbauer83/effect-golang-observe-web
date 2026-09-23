package inspect

// What the inspector puts on the wire.
//
// Its own shapes rather than the ones observe and web hold, for two reasons a
// page makes obvious. A span tree is recursive and a description of it would
// be too, so the tree is flattened and its shape carried in a depth -- which
// is also what a page renders: a list, indented. And a duration is
// microseconds as a whole number, because a page does arithmetic on it and
// "3.879µs" is prose.
//
// Described with schemas, so the inspector publishes its own contract from the
// same declarations that serve it. A tool whose API is guessed at is a tool
// somebody writes a client for twice.

// Span is one span, flattened: Depth carries what nesting carried.
//
// StartMicros is here because Effect's devtools schema puts a startTime on
// every span rather than a duration alone, and that is what a waterfall needs:
// a duration says how long, and only an offset says when. AgeMicros is the
// other half, for a span that has not ended and therefore has no duration to
// give.
type Span struct {
	ID       int64
	ParentID int64
	// Trace is the identity of the trace this span belongs to: the runtime's
	// span identity is a counter and names nothing outside one process, so a
	// trace referred to by a person or held across a restart needs this.
	// Empty for a span whose root is not in this reading.
	Trace  string
	Depth  int64
	Name   string
	Source string
	Status string
	// StartMicros is the offset from the earliest span in this reading, so a
	// page can place the bars without knowing what clock they came from.
	StartMicros int64
	Micros      int64
	// SelfMicros is the duration its children did not take, which is what
	// "hot" means: a span that spent all of itself inside one child is not
	// where the time went.
	SelfMicros int64
	AgeMicros  int64
	Open       bool
	Attributes []Attribute
	Events     []Event
}

// Attribute is one of a span's annotations, rendered.
//
// Rendered because the runtime's are slog.Attr and hold any value: putting
// that on a described wire would mean describing every kind slog can carry,
// and a page displays them either way. The rendering is Go's own, so what a
// reader sees is what the program annotated.
type Attribute struct {
	Key   string
	Value string
}

// Fiber is one fiber the runtime is running.
//
// The execution structure beside the logical one, and the one a program that
// has stopped responding is found through -- which is why ZIO's fiber dump
// leads with the age.
type Fiber struct {
	ID        int64
	ParentID  int64
	Depth     int64
	Operation string
	SpanID    int64
	AgeMicros int64
}

// LiveWork is what the runtime still holds, when it was asked to count.
//
// Zero for a runtime built without debug tracking, which is the default: the
// counters cost a pair of atomics per fiber and per resource, so the runtime
// does not keep them unless told to. Counted says which case a reading is,
// because two zeroes and "not counting" look identical otherwise.
type LiveWork struct {
	Counted   bool
	Fibers    int64
	Resources int64
}

// Event is one thing that happened inside a span.
//
// AtMicros is on the same basis as a span's StartMicros -- an offset from the
// earliest span in the reading -- so an event can be placed on the same
// timeline the spans are drawn on. Effect's devtools schema puts a start time
// on a span event for the same reason: an event with no position is a list
// item, and one with a position is a mark on a trace.
type Event struct {
	Kind      string
	Operation string
	Status    string
	Attempt   int64
	AtMicros  int64
}

// Measurement is one bounded label's counts and timings.
type Measurement struct {
	Kind      string
	Operation string
	Status    string
	Count     int64
	// MedianMicros, P95Micros, P99Micros and MaxMicros are zero where the
	// label measured no durations, which is the case for every event that
	// carries none.
	//
	// The three quantiles rather than the median alone, because a median is
	// the figure that hides what people complain about: work that answers in
	// three milliseconds almost always and in two seconds once in fifty has a
	// median saying three, and the two seconds is the whole of what anybody
	// noticed.
	MedianMicros int64
	P95Micros    int64
	P99Micros    int64
	MaxMicros    int64
	// DelayMicros is the total delay a retrying or repeating label waited.
	DelayMicros int64
}

// Route is one endpoint of the surface being served.
type Route struct {
	Method  string
	Path    string
	Summary string
	Status  int64
}

// Snapshot is one reading of everything the inspector shows.
type Snapshot struct {
	Time         string
	Process      Process
	Costs        []Cost
	Fibers       []Fiber
	LiveWork     LiveWork
	OpenSpans    []Span
	Trace        []Span
	LooseEvents  int64
	Measurements []Measurement
	Routes       []Route
	Drops        int64
}
