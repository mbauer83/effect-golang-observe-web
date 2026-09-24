package notebook

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
	schema.FieldAt("notes", schema.Int(), func(value *Report) *int { return &value.Notes }),
	schema.FieldAt("words", schema.Int(), func(value *Report) *int { return &value.Words }),
	schema.FieldAt("longest", schema.Text(), func(value *Report) *string { return &value.Longest }),
	schema.FieldAt("digest", schema.Text(), func(value *Report) *string { return &value.Digest }),
).WithDescription("Report is a summary of the notes.")

// Stages are the names the report's parts are measured under.
//
// Declared, because a metric label and a cost account are both bounded by a
// vocabulary and this is that vocabulary: the stages a reader of the inspector
// should be able to tell apart.
var Stages = []string{"read", "count", "digest", "rank"}

// NewReport summarises the notes in four measured stages.
//
// Each stage is one Measure: a span, a name and an account under one word.
// The costs differ on purpose -- digest allocates, count does not -- so the
// inspector's cost panel has something to rank and the hot stage is visibly
// not the slowest one.
func NewReport(store *Store, costs *process.Costs) task[Report] {
	return effect.Gen(func(do *effect.Do[effect.Unit, Refusal]) Report {
		notes := do.Await(process.Measure(costs, "read", store.All()))
		words := do.Await(process.Measure(costs, "count", countWords(notes)))
		digest := do.Await(process.Measure(costs, "digest", digestNotes(notes)))
		longest := do.Await(process.Measure(costs, "rank", rankTitles(notes)))
		return Report{
			Notes:   len(notes),
			Words:   words,
			Longest: longest,
			Digest:  digest,
		}
	})
}

// countWords walks the notes and allocates nothing worth measuring, which is
// what makes it the cheap stage.
func countWords(notes []Note) task[int] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, int] {
		words := 0
		for _, note := range notes {
			words += len(strings.Fields(note.Body)) + len(strings.Fields(note.Title))
		}
		return effect.ExitSuccess[Refusal](words)
	})
}

// digestNotes builds a string per note and throws most of it away, which is the
// shape of a great deal of real work and the reason a cost panel is worth
// having: it is not the slowest stage and it is the one allocating.
func digestNotes(notes []Note) task[string] {
	return effect.From(func(context.Context, effect.Unit) effect.Exit[Refusal, string] {
		var builder strings.Builder
		for round := range 24 {
			for _, note := range notes {
				builder.WriteString(strings.ToUpper(note.Title))
				builder.WriteString(":")
				builder.WriteString(strings.Repeat(note.Body, 1+round%3))
				builder.WriteString(" ")
			}
		}
		digest := builder.String()
		if len(digest) > 24 {
			digest = digest[:24]
		}
		return effect.ExitSuccess[Refusal](digest)
	})
}

// rankTitles finds the longest title, and refuses twice before it does.
//
// A retry inside a stage, so the trace shows the attempts as events on the
// stage's own bar rather than as a mystery in the total.
func rankTitles(notes []Note) task[string] {
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
