package inspect

// Reading the process's memory and compute into the shapes a chart is drawn
// from.

import (
	"math"
	"time"

	"github.com/mbauer83/effect-golang-observe/process"
)

// sampled takes a reading, keeps it in the series, and reads the whole series
// out as gauges, a window and a set of points.
//
// Taking the reading here is what makes the chart's resolution the rate the
// inspector is looked at. Nothing samples on a schedule: a sampler would be a
// goroutine this would have to own, and the runtime deliberately does not
// spawn those for a capability.
func sampled(series *process.Series) Process {
	latest := series.Sample()
	readings := series.Readings()

	shown := Process{
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
	if recent, measurable := series.Recent(); measurable {
		shown.OverMicros = recent.Over.Microseconds()
		shown.AllocatedBytes = int64(recent.AllocatedBytes)
		shown.BytesPerSecond = recent.AllocationRate()
		shown.Busy = recent.Busy()
		shown.Collecting = recent.Collecting()
	}
	return shown
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
			point.AtMicros = reading.Taken.Sub(readings[0].Taken).Microseconds()
			step := process.Between(readings[index-1], reading)
			point.BytesPerSecond = step.AllocationRate()
			point.Busy = step.Busy()
		}
		points = append(points, point)
	}
	return points
}

func costsOf(accounted []process.Cost, origin time.Time) []Cost {
	shown := make([]Cost, 0, len(accounted))
	for _, cost := range accounted {
		shown = append(shown, Cost{
			Name:             cost.Name,
			Times:            int64(cost.Times),
			AllocatedDuring:  int64(cost.AllocatedDuring),
			PerRunBytes:      int64(cost.PerRun()),
			ObjectsPerRun:    int64(cost.ObjectsPerRun()),
			MeanObjectBytes:  int64(cost.MeanObjectBytes()),
			CPUSecondsDuring: cost.CPUSecondsDuring,
			LongestMicros:    cost.Longest.Microseconds(),
			Collections:      int64(cost.Collections),
			Sizes:            sizesOf(cost.Spread.Banded()),
			Runs:             runsOf(cost.Runs, origin),
		})
	}
	return shown
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
	shown := make([]Run, 0, len(runs))
	if origin.IsZero() {
		return shown
	}
	for _, run := range runs {
		if run.Ended.Before(origin) {
			continue
		}
		shown = append(shown, Run{
			EndedMicros: run.Ended.Sub(origin).Microseconds(),
			Micros:      run.Change.Over.Microseconds(),
			Bytes:       int64(run.Change.AllocatedBytes),
			Objects:     int64(run.Change.AllocatedObjects),
		})
	}
	return shown
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
