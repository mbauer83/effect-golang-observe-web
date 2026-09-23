package inspect

// What the process spent, as a client reads it.
//
// Its own file because it is its own subject: the shapes beside this one are
// what the runtime did -- spans, fibers, events -- and these are what the
// machine underneath it paid for them. A reader chasing an allocation and a
// reader chasing a slow span are looking at different tables.

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
	AllocBytes     int64
	BytesPerSecond float64
	Busy           float64
	GCShare        float64

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
	Name        string
	Times       int64
	BytesDuring int64
	PerRunBytes int64
	// ObjectsPerRun is how many allocations a run made and MeanObjectBytes
	// their average size. In Go the count is usually the more actionable of
	// the two: an allocation costs tens of nanoseconds and a pointer for the
	// collector to chase whatever its size.
	ObjectsPerRun   int64
	MeanObjectBytes int64
	// BytesP50, AllocatedP95Bytes and AllocatedP99Bytes are where
	// this name's runs fell, and ObjectsP95 how many allocations the worst one
	// in twenty made.
	//
	// Exact over the runs the account kept rather than a bound from a
	// histogram: these are the measurements sorted, so each is a run that
	// happened. SampleSize says how many they are drawn from, because a
	// ninety-ninth percentile over three runs is a sentence with no content.
	BytesP50         int64
	BytesP95         int64
	BytesP99         int64
	ObjectsP95       int64
	SampleSize       int64
	CPUSecondsDuring float64
	LongestMicros    int64
	Collections      int64
	// Sizes are the size classes the allocations fell into, smallest first,
	// or empty when the account was not told to keep them. The disclosed
	// layer: the bytes and the count first, what shapes they were second.
	Sizes []SizeClass
	// Runs are the recent runs of this name, newest first: what one span did,
	// where the figures above are what the name costs on average. A span is
	// matched to the run whose window ended inside it.
	Runs []Run
}

// Run is one run of a name: when its window ended, on the same timeline the
// spans are on, and what the process did during it.
//
// Which is how a span gets a figure of its own. The account is keyed by name
// and averaged over every run of it, so a trace nobody is running any more
// would keep changing its numbers -- and the run that allocated ten times the
// usual amount would be invisible in the average, which is the run worth
// finding.
type Run struct {
	// EndMicros is the offset from the earliest span in this reading, so it
	// can be compared against a span's window without either side knowing
	// whose clock it came from. Zero when there are no spans to measure from.
	EndMicros int64
	Micros    int64
	Bytes     int64
	Objects   int64
}

// SizeClass is one of Go's allocation size classes and how many allocations
// fell in it.
type SizeClass struct {
	// AtMostBytes is the class's upper edge, and zero for the widest class,
	// which has none.
	AtMostBytes int64
	Count       int64
}
