package acceptance

// The standard arrangement: traces to a collector, a scrape, and the
// inspector, from one set of settings.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mbauer83/effect-golang-observe-web/observability"
	"github.com/mbauer83/effect-golang-schema/schema"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

func greeting() web.Route[effect.Unit, error] {
	return web.Handle(web.GET("/greetings/{name}", web.PathParam("name", schema.Text()), web.Returns(http.StatusOK, schema.Text())),
		func(name string) effect.Effect[effect.Unit, error, string] {
			return effect.Succeed[effect.Unit, error]("hello " + name)
		})
}

func rejectPlainly(err error) web.Response { return web.Text(http.StatusBadRequest, err.Error()) }

func TestOneSetOfSettingsWatchesTheWholeService(t *testing.T) {
	var batches atomic.Int32
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, _ := io.ReadAll(r.Body); strings.Contains(string(body), "GET /greetings/{name}") {
			batches.Add(1)
		}
	}))
	defer collector.Close()

	routes := []web.Route[effect.Unit, error]{greeting()}
	watch, err := observability.New(observability.Settings{Service: "greeter", TracesTo: collector.URL + "/v1/traces",
		MetricsAt: "/metrics", Namespace: "greeter", InspectAt: "/inspect"}, web.DeclarationsOf(routes...))
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := watch.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	surface, err := observability.Surface(watch, rejectPlainly, routes...)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := web.NewAdapter(runtime, effect.Unit{}, func(error) web.Response { return web.Text(500, "") })
	if err != nil {
		t.Fatal(err)
	}
	service := httptest.NewServer(adapter.Handler(surface.Handler()))
	defer service.Close()

	program := effect.Scoped(func(scope effect.Scope) effect.Effect[effect.Unit, error, effect.Unit] {
		return observability.Deliver[effect.Unit, error](watch, scope).AndThen(
			effect.From(func(context.Context, effect.Unit) effect.Exit[error, effect.Unit] {
				bodyAt(t, service.URL+"/greetings/ada")
				scrape := bodyAt(t, service.URL+"/metrics")
				if !strings.Contains(scrape, `greeter_runtime_events_total{`) || !strings.Contains(scrape, `operation="GET /greetings/{name}"`) {
					t.Errorf("expected the route's own series in the scrape, got\n%s", scrape)
				}
				if page := bodyAt(t, service.URL+"/inspect"); !strings.Contains(page, "<html") {
					t.Error("expected the inspector's page")
				}
				return effect.ExitSuccess[error](effect.Unit{})
			}))
	})
	if err := effect.RunUntilStopped(runtime, program); err != nil {
		t.Fatal(err)
	}
	if batches.Load() == 0 {
		t.Error("expected the request's span delivered when the scope closed")
	}
}

func TestNothingNamedIsNothingWatched(t *testing.T) {
	routes := []web.Route[effect.Unit, error]{greeting()}
	watch, err := observability.New(observability.Settings{}, web.DeclarationsOf(routes...))
	if err != nil {
		t.Fatal(err)
	}
	surface, err := observability.Surface(watch, rejectPlainly, routes...)
	if err != nil || len(surface.Declarations()) != 1 {
		t.Errorf("expected only the program's own route, got %d, %v", len(surface.Declarations()), err)
	}
}

func bodyAt(t *testing.T, address string) string {
	t.Helper()
	response, err := http.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	return string(body)
}
