package inspect

// Reading the process's memory and compute into the shapes a chart is drawn
// from.

import (
	"math"
	"time"

	"github.com/mbauer83/effect-golang-observe/process"
)

// sampleProcess takes a reading, keeps it in the series, and reads the whole series
// out as gauges, a window and a set of points.
//
// Taking the reading here is what makes the chart's resolution the rate the
// inspector is looked at. Nothing samples on a schedule: a sampler would be a
// goroutine this would have to own, and the runtime deliberately does not
// spawn those for a capability.
func sampleProcess(series *process.Series) Process {
	latest := series.Sample()
	readings := series.Readings()

	result := Process{
		Sampled:     true,
		HeapBytes:   int64(latest.HeapBytes),
		HeapObjects: int64(latest.HeapObjects),
		LiveBytes:   int64(latest.LiveBytes),
		GoalBytes:   int64(latest.GoalBytes),
		StackBytes:  int64(latest.StackBytes),
		TotalBytes:  int64(latest.TotalBytes),
		Goroutines:  int64(latest.Goroutines),
		Running:     int64(latest.Running),
		Runnable:    int64(latest.Runnable),
		Waiting:     int64(latest.Waiting),
		Threads:     int64(latest.Threads),
		GCCycles:    int64(latest.GCCycles),
		Points:      pointsOf(readings),
	}
	if recent, measurable := series.Change(); measurable {
		result.OverMicros = recent.Duration.Microseconds()
		result.AllocBytes = int64(recent.AllocBytes)
		result.BytesPerSecond = recent.AllocationRate()
		result.Busy = recent.Busy()
		result.GCShare = recent.GCShare()
	}
	return result
}

// pointsOf turns the readings into points, each offset from the first.
//
// The rates are between neighbours rather than cumulative, because a
// cumulative counter drawn as a line is a slope nobody can compare: two
// programs allocating at the same rate have different lines depending on how
// long they have been running.
func pointsOf(readings []process.Reading) []Point {
	points := make([]Point, 0, len(readings))
	for index, reading := range readings {
		point := Point{
			HeapBytes:  int64(reading.HeapBytes),
			Goroutines: int64(reading.Goroutines),
			Runnable:   int64(reading.Runnable),
			StackBytes: int64(reading.StackBytes),
			GoalBytes:  int64(reading.GoalBytes),
		}
		if index > 0 {
			point.AtMicros = reading.Time.Sub(readings[0].Time).Microseconds()
			step := process.Diff(readings[index-1], reading)
			point.BytesPerSecond = step.AllocationRate()
			point.Busy = step.Busy()
		}
		points = append(points, point)
	}
	return points
}

func costsOf(costs []process.Cost, origin time.Time) []Cost {
	result := make([]Cost, 0, len(costs))
	for _, cost := range costs {
		result = append(result, Cost{
			Name:             cost.Name,
			Times:            int64(cost.Times),
			BytesDuring:      int64(cost.BytesDuring),
			PerRunBytes:      int64(cost.PerRun()),
			ObjectsPerRun:    int64(cost.ObjectsPerRun()),
			MeanObjectBytes:  int64(cost.MeanObjectBytes()),
			BytesP50:         int64(cost.BytesAt(0.5)),
			BytesP95:         int64(cost.BytesAt(0.95)),
			BytesP99:         int64(cost.BytesAt(0.99)),
			ObjectsP95:       int64(cost.ObjectsAt(0.95)),
			SampleSize:       int64(cost.RunCount()),
			CPUSecondsDuring: cost.CPUSecondsDuring,
			LongestMicros:    cost.Longest.Microseconds(),
			Collections:      int64(cost.Collections),
			Sizes:            sizesOf(cost.Spread.Bands()),
			Runs:             runsOf(cost.Runs, origin),
		})
	}
	return result
}

// runsOf reads the recent runs onto the wire, offset from the same origin the
// spans are, so the page can match a run to the span whose window it ended in
// without either side being given a clock.
//
// A run that ended before the earliest span in this reading is left out: the
// span it belonged to has already gone, so nothing in this reading can be
// attributed to it -- and sending it would grow with the account's memory
// rather than with what is being looked at.
func runsOf(runs []process.Run, origin time.Time) []Run {
	result := make([]Run, 0, len(runs))
	if origin.IsZero() {
		return result
	}
	for _, run := range runs {
		if run.EndTime.Before(origin) {
			continue
		}
		result = append(result, Run{
			EndMicros: run.EndTime.Sub(origin).Microseconds(),
			Micros:    run.Change.Duration.Microseconds(),
			Bytes:     int64(run.Change.AllocBytes),
			Objects:   int64(run.Change.AllocObjects),
		})
	}
	return result
}

// sizesOf reads the size bands onto the wire.
//
// Banded, not raw: Go has sixty-eight size classes and a busy program touches
// nearly all of them, so the raw list is a wall rather than a disclosure --
// and sending eleven names times sixty-eight classes per refresh would be
// paying for the wall as well as reading it.
//
// The widest class has no upper bound -- it is reported as +Inf, which JSON
// has no number for -- so it crosses as a zero and the page renders it as
// "larger". A zero upper edge is impossible for a real class, so nothing is
// ambiguous about it.
func sizesOf(spread process.Spread) []SizeClass {
	classes := make([]SizeClass, 0, len(spread.Classes))
	for _, class := range spread.Classes {
		edge := int64(0)
		if !math.IsInf(class.AtMost, 1) {
			edge = int64(class.AtMost)
		}
		classes = append(classes, SizeClass{AtMostBytes: edge, Count: int64(class.Count)})
	}
	return classes
}
