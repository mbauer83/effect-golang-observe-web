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
	Depth    int64
	Name     string
	Source   string
	Status   string
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

// Owned is what the runtime still holds, when it was asked to count.
//
// Zero for a runtime built without debug tracking, which is the default: the
// counters cost a pair of atomics per fiber and per resource, so the runtime
// does not keep them unless told to. Counted says which case a reading is,
// because two zeroes and "not counting" look identical otherwise.
type Owned struct {
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
	// MedianMicros and MaxMicros are zero where the label measured no
	// durations, which is the case for every event that carries none.
	MedianMicros int64
	MaxMicros    int64
	// WaitedMicros is the total delay a retrying or repeating label waited.
	WaitedMicros int64
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
	TakenAt      string
	Process      Process
	Costs        []Cost
	Fibers       []Fiber
	Owned        Owned
	OpenSpans    []Span
	Trace        []Span
	LooseEvents  int64
	Measurements []Measurement
	Routes       []Route
	Dropped      int64
}

// Process is what the program is spending: gauges for now, a window for the
// rate, and the points a chart is drawn from.
//
// Every number is process-wide. Go has no per-goroutine allocation counter and
// no per-goroutine CPU clock, so a figure attributed to one span would be
// invented -- which is why the per-name accounts below are named for a window
// rather than for an attribution.
type Process struct {
	Sampled bool

	HeapBytes   int64
	HeapObjects int64
	LiveBytes   int64
	// GoalBytes is the heap size the next collection is aiming at, so
	// LiveBytes against it says how close one is.
	GoalBytes  int64
	StackBytes int64
	TotalBytes int64

	// Goroutines and the three below it are the scheduler's own breakdown.
	// Runnable is the interesting one: ready and not running means waiting for
	// a thread.
	Goroutines int64
	Running    int64
	Runnable   int64
	Waiting    int64
	Threads    int64
	GCCycles   int64

	// The window: how long it covered, what was allocated in it, and the two
	// shares that say whether the program is busy and whether it is spending
	// that time collecting.
	OverMicros     int64
	AllocatedBytes int64
	BytesPerSecond float64
	Busy           float64
	Collecting     float64

	Points []Point
}

// Point is one reading on a chart, offset from the first.
type Point struct {
	AtMicros   int64
	HeapBytes  int64
	Goroutines int64
	Runnable   int64
	StackBytes int64
	GoalBytes  int64
	// BytesPerSecond and Busy are for the window since the previous point,
	// which is what makes them readable as a line: a cumulative counter drawn
	// directly is a slope nobody can compare.
	BytesPerSecond float64
	Busy           float64
}

// Cost is what the process spent while one route's work ran.
//
// AllocatedDuring is the process's allocation during the work, so on a busy
// program it includes whatever else was running. The name carries the caveat
// because a reader who takes it for attribution will draw the wrong
// conclusion, and a paragraph elsewhere will not stop them.
type Cost struct {
	Name             string
	Times            int64
	AllocatedDuring  int64
	PerRunBytes      int64
	CPUSecondsDuring float64
	LongestMicros    int64
	Collections      int64
}
