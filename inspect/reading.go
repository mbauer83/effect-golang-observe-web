package inspect

// Reading the process's memory and compute into the shapes a chart is drawn
// from.

import (
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

func costsOf(accounted []process.Cost) []Cost {
	shown := make([]Cost, 0, len(accounted))
	for _, cost := range accounted {
		shown = append(shown, Cost{
			Name:             cost.Name,
			Times:            int64(cost.Times),
			AllocatedDuring:  int64(cost.AllocatedDuring),
			PerRunBytes:      int64(cost.PerRun()),
			CPUSecondsDuring: cost.CPUSecondsDuring,
			LongestMicros:    cost.Longest.Microseconds(),
			Collections:      int64(cost.Collections),
		})
	}
	return shown
}
