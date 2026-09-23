package inspected

// A route with something to decompose.
//
// The other three answer from a Ref and are over in microseconds, which makes
// a timeline of one bar: there is nothing to see because there is nothing
// underneath. This one has stages, the stages have different costs, and one of
// them retries -- so a trace of it has depth, a waterfall has something to
// draw, and the hot stage is a different one from the slow stage.

import (
	"context"
	"strings"

	"github.com/mbauer83/effect-golang-observe/process"
	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
	"github.com/mbauer83/effect-golang/experimental/direct"
)

// Report is what the route answers with.
type Report struct {
	Notes   int
	Words   int
	Longest string
	Digest  string
}

// ReportSchema describes it.
var ReportSchema = schema.Struct[Report]("Report",
	schema.FieldOf("notes", schema.Int(),
		func(value Report) int { return value.Notes },
		func(value *Report, field int) { value.Notes = field }),
	schema.FieldOf("words", schema.Int(),
		func(value Report) int { return value.Words },
		func(value *Report, field int) { value.Words = field }),
	schema.FieldOf("longest", schema.Text(),
		func(value Report) string { return value.Longest },
		func(value *Report, field string) { value.Longest = field }),
	schema.FieldOf("digest", schema.Text(),
		func(value Report) string { return value.Digest },
		func(value *Report, field string) { value.Digest = field }),
).Documented("Report is a summary of the notes.")

// Stages are the names the report's parts are measured under.
//
// Declared, because a metric label and a cost account are both bounded by a
// vocabulary and this is that vocabulary: the stages a reader of the inspector
// should be able to tell apart.
var Stages = []string{"read", "count", "digest", "rank"}

// Reported summarises the notes in four measured stages.
//
// Each stage is one Measured: a span, a name and an account under one word.
// The costs differ on purpose -- digest allocates, count does not -- so the
// inspector's cost panel has something to rank and the hot stage is visibly
// not the slowest one.
func Reported(store *Store, costs *process.Costs) storing[Report] {
	return direct.Run(func(do *direct.Do[effect.Unit, Refusal]) Report {
		held := do.Await(process.Measured(costs, "read", store.All()))
		counted := do.Await(process.Measured(costs, "count", counting(held)))
		digest := do.Await(process.Measured(costs, "digest", digesting(held)))
		longest := do.Await(process.Measured(costs, "rank", ranking(held)))
		return Report{
			Notes:   len(held),
			Words:   counted,
			Longest: longest,
			Digest:  digest,
		}
	})
}

// counting walks the notes and allocates nothing worth measuring, which is
// what makes it the cheap stage.
func counting(notes []Note) storing[int] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, int] {
		words := 0
		for _, note := range notes {
			words += len(strings.Fields(note.Body)) + len(strings.Fields(note.Title))
		}
		return effect.ExitSuccess[Refusal](words)
	})
}

// digesting builds a string per note and throws most of it away, which is the
// shape of a great deal of real work and the reason a cost panel is worth
// having: it is not the slowest stage and it is the one allocating.
func digesting(notes []Note) storing[string] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, string] {
		var built strings.Builder
		for round := range 24 {
			for _, note := range notes {
				built.WriteString(strings.ToUpper(note.Title))
				built.WriteString(":")
				built.WriteString(strings.Repeat(note.Body, 1+round%3))
				built.WriteString(" ")
			}
		}
		digest := built.String()
		if len(digest) > 24 {
			digest = digest[:24]
		}
		return effect.ExitSuccess[Refusal](digest)
	})
}

// ranking finds the longest title, and refuses twice before it does.
//
// A retry inside a stage, so the trace shows the attempts as events on the
// stage's own bar rather than as a mystery in the total.
func ranking(notes []Note) storing[string] {
	attempts := 0
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, string] {
		attempts++
		if attempts < 3 {
			return effect.ExitFailure[Refusal, string](
				Refusal{Kind: "ranking was busy", Title: ""})
		}
		longest := ""
		for _, note := range notes {
			if len(note.Title) > len(longest) {
				longest = note.Title
			}
		}
		return effect.ExitSuccess[Refusal](longest)
	}).RetryN(3)
}
