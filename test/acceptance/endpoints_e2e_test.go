package acceptance

// What the inspector serves beside the snapshot, and what it does not disturb.
//
// Its own file because these are claims about the surface rather than about
// the telemetry: the page reaches nowhere, the contract describes the reading,
// the runtime's counts arrive, and the program's own refusals stay the
// program's.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mbauer83/effect-golang-observe-web/examples/notebook"
	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func TestTheInspectorServesItsPageAndItsOwnContract(t *testing.T) {
	client := serveExample(t)

	page := fetch(t, client, http.MethodGet, inspect.DefaultAt)
	if page.Status != http.StatusOK {
		t.Fatalf("expected the page, got %d", page.Status)
	}
	if kind := page.Header.Get("Content-Type"); !strings.HasPrefix(kind, "text/html") {
		t.Fatalf("expected HTML, got %q", kind)
	}
	// One document and no external asset: a tool that only works where the
	// internet is reachable is not a debugging tool.
	if body := string(page.Entity); strings.Contains(body, "https://") {
		t.Error("the page reaches outside the process for an asset")
	}

	contract := fetch(t, client, http.MethodGet, inspect.DefaultAt+"/openapi.json")
	if contract.Status != http.StatusOK {
		t.Fatalf("expected the contract, got %d", contract.Status)
	}
	// It describes the snapshot the page reads, which is what makes a client
	// of the inspector a client of a contract.
	for _, want := range []string{"Snapshot", "startMicros", "ageMicros", "/snapshot"} {
		if !strings.Contains(string(contract.Entity), want) {
			t.Errorf("the contract does not mention %q", want)
		}
	}
}

func TestTheRuntimesOwnCountsAreReported(t *testing.T) {
	snapshot := readSnapshot(t, serveExample(t))
	if !snapshot.LiveWork.Counted {
		t.Fatal("expected the runtime's counts, which this runtime was built to keep")
	}
	// Sampled while serving, so the serving fiber and the server's resources
	// are held: a zero here would mean the sampling missed the program.
	if snapshot.LiveWork.Fibers == 0 && snapshot.LiveWork.Resources == 0 {
		t.Fatalf("expected the serving work to be owned, got %+v", snapshot.LiveWork)
	}
}

func TestTheProgramsOwnRefusalsStayItsOwn(t *testing.T) {
	// The inspector never becomes the program's failure. A conflict is still a
	// 409 from the program's own boundary mapping, with the inspector mounted
	// on the same surface.
	client := serveExample(t)
	request, err := web.WithEntity(web.ClientRequest{}, notebook.NoteSchema,
		notebook.Note{Title: "First", Body: "again"})
	if err != nil {
		t.Fatal(err)
	}
	exit := effect.Run(context.Background(), effect.Unit{},
		web.Call[effect.Unit](client, notebook.AddNote, request))

	cause, failed := exit.Cause()
	if !failed {
		t.Fatalf("expected the duplicate title refused, got %v", exit)
	}
	fault, _ := cause.Failure()
	var refusal web.Refusal
	if !errors.As(fault, &refusal) || refusal.Status != http.StatusConflict {
		t.Fatalf("expected a 409 from the program's own mapping, got %v", fault)
	}
}
