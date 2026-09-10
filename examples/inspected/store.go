// Package inspected is a small web program with the inspector mounted, which
// is the whole point demonstrated: the tooling lives inside the program it
// watches, on the same listener, holding the same telemetry.
package inspected

import (
	"net/http"
	"slices"

	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang/effect"
)

// Note is one note the program holds.
type Note struct {
	Title string
	Body  string
}

// NoteSchema describes a note, and is what serves both directions.
var NoteSchema = schema.Struct[Note]("Note",
	schema.FieldOf("title", schema.Text().Constrained(schema.MinLength(1), schema.MaxLength(80)),
		func(value Note) string { return value.Title },
		func(value *Note, field string) { value.Title = field }).
		Documented("Title identifies the note."),
	schema.FieldOf("body", schema.Text().Constrained(schema.MaxLength(400)),
		func(value Note) string { return value.Body },
		func(value *Note, field string) { value.Body = field }),
).Documented("Note is one note.")

// NotesSchema is the collection.
var NotesSchema = schema.List(NoteSchema)

// NoteTitle is the title as a path parameter reads it, which is the same rule
// the entity's own field carries.
var NoteTitle = schema.Text().Constrained(schema.MinLength(1), schema.MaxLength(80))

// Refusal is the program's own failure, which the inspector never becomes.
type Refusal struct {
	Kind  string
	Title string
}

func (refusal Refusal) Error() string {
	return refusal.Kind + ": " + refusal.Title
}

// Store keeps the notes in a Ref, so its state cannot exist before something
// interprets the description that makes it.
type Store struct {
	held effect.Ref[[]Note]
}

type storing[A any] = effect.Effect[effect.Unit, Refusal, A]

// NewStore builds a store holding the notes given.
func NewStore(notes ...Note) effect.Effect[effect.Unit, effect.Never, *Store] {
	return effect.NewRef[effect.Unit](slices.Clone(notes)).
		Map(func(held effect.Ref[[]Note]) *Store { return &Store{held: held} })
}

// All reads every note.
func (store *Store) All() storing[[]Note] {
	return widened(store.held.Get[effect.Unit]().Map(slices.Clone))
}

// Add keeps a note, refusing a title the store already holds.
func (store *Store) Add(note Note) storing[Note] {
	added := effect.Modify[effect.Unit](store.held, func(held []Note) ([]Note, bool) {
		if slices.ContainsFunc(held, sameTitle(note.Title)) {
			return held, false
		}
		return append(slices.Clone(held), note), true
	})
	return widened(added).FlatMap(func(added bool) storing[Note] {
		operations := effect.For[effect.Unit, Refusal]()
		if !added {
			return operations.Fail[Note](Refusal{Kind: "held already", Title: note.Title})
		}
		return operations.Succeed(note)
	})
}

// Find reads one note by title.
func (store *Store) Find(title string) storing[Note] {
	return store.All().FlatMap(func(held []Note) storing[Note] {
		operations := effect.For[effect.Unit, Refusal]()
		at := slices.IndexFunc(held, sameTitle(title))
		if at < 0 {
			return operations.Fail[Note](Refusal{Kind: "no such note", Title: title})
		}
		return operations.Succeed(held[at])
	})
}

// StatusFor maps the program's vocabulary to statuses, at the boundary and
// nowhere else.
func StatusFor(refusal Refusal) int {
	if refusal.Kind == "held already" {
		return http.StatusConflict
	}
	return http.StatusNotFound
}

func sameTitle(title string) func(Note) bool {
	return func(note Note) bool { return note.Title == title }
}

// widened puts an effect that cannot fail into the channel that can, which is
// what lets a Ref's operations compose with the store's own refusals.
func widened[A any](fx effect.Effect[effect.Unit, effect.Never, A]) storing[A] {
	return effect.For[effect.Unit, Refusal]().WidenError(fx)
}
