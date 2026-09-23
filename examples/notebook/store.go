// Package notebook is a small web program with the inspector mounted, which
// is the whole point demonstrated: the tooling lives inside the program it
// watches, on the same listener, holding the same telemetry.
package notebook

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
	schema.FieldOf("title", schema.Text().Check(schema.MinLength(1), schema.MaxLength(80)),
		func(value Note) string { return value.Title },
		func(value *Note, field string) { value.Title = field }).
		WithDescription("Title identifies the note."),
	schema.FieldOf("body", schema.Text().Check(schema.MaxLength(400)),
		func(value Note) string { return value.Body },
		func(value *Note, field string) { value.Body = field }),
).WithDescription("Note is one note.")

// NotesSchema is the collection.
var NotesSchema = schema.List(NoteSchema)

// NoteTitle is the title as a path parameter reads it, which is the same rule
// the entity's own field carries.
var NoteTitle = schema.Text().Check(schema.MinLength(1), schema.MaxLength(80))

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
	notes effect.Ref[[]Note]
}

type task[A any] = effect.Effect[effect.Unit, Refusal, A]

// NewStore builds a store holding the notes given.
func NewStore(notes ...Note) effect.Effect[effect.Unit, effect.Never, *Store] {
	return effect.NewRef[effect.Unit](slices.Clone(notes)).
		Map(func(ref effect.Ref[[]Note]) *Store { return &Store{notes: ref} })
}

// All reads every note.
func (store *Store) All() task[[]Note] {
	return widen(store.notes.Get[effect.Unit]().Map(slices.Clone))
}

// Add keeps a note, refusing a title the store already holds.
func (store *Store) Add(note Note) task[Note] {
	insert := effect.Modify[effect.Unit](store.notes, func(notes []Note) ([]Note, bool) {
		if slices.ContainsFunc(notes, sameTitle(note.Title)) {
			return notes, false
		}
		return append(slices.Clone(notes), note), true
	})
	return widen(insert).FlatMap(func(added bool) task[Note] {
		operations := effect.For[effect.Unit, Refusal]()
		if !added {
			return operations.Fail[Note](Refusal{Kind: "held already", Title: note.Title})
		}
		return operations.Succeed(note)
	})
}

// Find reads one note by title.
func (store *Store) Find(title string) task[Note] {
	return store.All().FlatMap(func(ref []Note) task[Note] {
		operations := effect.For[effect.Unit, Refusal]()
		at := slices.IndexFunc(ref, sameTitle(title))
		if at < 0 {
			return operations.Fail[Note](Refusal{Kind: "no such note", Title: title})
		}
		return operations.Succeed(ref[at])
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

// widen puts an effect that cannot fail into the channel that can, which is
// what lets a Ref's operations compose with the store's own refusals.
func widen[A any](fx effect.Effect[effect.Unit, effect.Never, A]) task[A] {
	return effect.For[effect.Unit, Refusal]().WidenError(fx)
}
