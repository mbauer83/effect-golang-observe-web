// Package observability is the standard arrangement for watching a service
// built on effect-golang-web: traces exported over OTLP, runtime metrics for a
// Prometheus scrape, and the runtime inspector, each off until a deployment
// names it. With nothing named, no observer is attached and the runtime emits
// nothing.
//
//	watch, err := observability.New(settings.Observability, web.DeclarationsOf(routes...))
//	runtime, err := watch.Runtime()
//	surface, err := observability.Surface(watch, reject, routes...)
//	program := observability.Deliver[effect.Unit, fault.Fault](watch, scope).AndThen(serving)
//
// A program that assembles its own surface adds Routes to it and passes the
// assembled surface through Instrument, which is what Surface does.
package observability

import (
	"github.com/mbauer83/effect-golang-observe-export/otlp"
	"github.com/mbauer83/effect-golang-observe-export/prometheus"
	"github.com/mbauer83/effect-golang-observe-web/inspect"
	"github.com/mbauer83/effect-golang-observe/metrics"
	"github.com/mbauer83/effect-golang-observe/observe"
	"github.com/mbauer83/effect-golang-web/web"
	"github.com/mbauer83/effect-golang/effect"
)

// Settings are what a deployment decides about watching one service. Each
// part is off while it is empty.
type Settings struct {
	// Service names the program on every span and series.
	Service string
	// TracesTo is the collector's OTLP/HTTP traces endpoint, as
	// http://collector:4318/v1/traces.
	TracesTo string
	// MetricsAt is the path Prometheus scrapes, as /metrics.
	MetricsAt string
	// Namespace prefixes every metric name; "runtime" when empty.
	Namespace string
	// InspectAt is the path the runtime inspector is mounted at.
	InspectAt string
}

// Watch observes one service. It is made from the settings and the routes'
// declarations before the runtime, because the observers are the runtime's,
// and is then attached to the surface and to the program's scope.
type Watch struct {
	settings  Settings
	observers []effect.Observer
	exporter  *otlp.Exporter
	collector *metrics.Collector
	inspector *inspect.Telemetry
}

// New is what the settings ask to watch. The routes' declarations name the
// series and spans, so a route added to the surface is measured under its own
// name without being listed a second time.
func New(settings Settings, routes []web.Declaration) (*Watch, error) {
	watch := &Watch{settings: settings}
	names, err := vocabulary(settings, routes)
	if err != nil {
		return nil, err
	}
	if settings.TracesTo != "" {
		exporter, err := otlp.NewExporter(otlp.Settings{Service: settings.Service, Endpoint: settings.TracesTo})
		if err != nil {
			return nil, err
		}
		watch.exporter, watch.observers = exporter, append(watch.observers, exporter)
	}
	if settings.MetricsAt != "" {
		watch.collector = metrics.NewCollector(metrics.NewVocabulary(names...))
		watch.observers = append(watch.observers, watch.collector)
	}
	if settings.InspectAt != "" {
		inspector, observer, err := inspect.NewTelemetry(inspect.TelemetryConfig{Names: names})
		if err != nil {
			return nil, err
		}
		watch.inspector, watch.observers = inspector, append(watch.observers, observer)
	}
	return watch, nil
}

// Runtime is a runtime with what the settings ask to watch attached, and the
// options given.
func (watch *Watch) Runtime(options ...effect.RuntimeOption) (*effect.Runtime, error) {
	if len(watch.observers) > 0 {
		options = append(options, effect.WithObserver(observe.Fanout(watch.observers...)))
	}
	runtime, err := effect.NewRuntime(options...)
	if err == nil && watch.inspector != nil {
		watch.inspector.LiveWork = runtime.LiveWork
	}
	return runtime, err
}

// Surface assembles the program's routes together with the scrape and the
// inspector the settings name, and instruments them. reject builds the
// response to a request the codecs refused, as in web.NewRoutesWithRejection.
func Surface[R, E any](watch *Watch, reject func(error) web.Response, routes ...web.Route[R, E]) (web.Routes[R, E], error) {
	served, err := Routes[R, E](watch)
	if err != nil {
		return web.Routes[R, E]{}, err
	}
	surface, err := web.NewRoutesWithRejection(reject, append(routes, served...)...)
	if err != nil {
		return surface, err
	}
	return Instrument(watch, surface), nil
}

// Routes are the routes watching serves: the scrape and the inspector, as the
// settings name them.
func Routes[R, E any](watch *Watch) ([]web.Route[R, E], error) {
	var routes []web.Route[R, E]
	if watch.collector != nil {
		namespace := watch.settings.Namespace
		if namespace == "" {
			namespace = "runtime"
		}
		routes = append(routes, prometheus.Route[R, E](watch.settings.MetricsAt, watch.collector,
			prometheus.Exposition{Namespace: namespace, Labels: map[string]string{"service": watch.settings.Service}}))
	}
	if watch.inspector != nil {
		inspector, err := inspect.Routes[R, E](watch.inspector, watch.settings.InspectAt)
		if err != nil {
			return nil, err
		}
		routes = append(routes, inspector...)
	}
	return routes, nil
}

// Instrument traces every request of an assembled surface as a span named for
// its route. With the inspector, each phase of a request -- decoding, the
// handler, encoding -- is also measured into its account, and the inspector is
// shown the surface.
func Instrument[R, E any](watch *Watch, surface web.Routes[R, E]) web.Routes[R, E] {
	switch {
	case watch.inspector != nil:
		watch.inspector.Surface = surface.Declarations()
		return surface.WithPhaseSampler(inspect.Sampler(watch.inspector.Costs)).
			WithMiddleware(inspect.Tracer[R, E](watch.inspector.Costs))
	case len(watch.observers) > 0:
		return surface.WithMiddleware(web.RouteSpans[R, E]())
	default:
		return surface
	}
}

// vocabulary names the series and accounts: the program's routes, and with
// the inspector the phases of a request and the inspector's own routes, whose
// traffic would otherwise count as unnamed.
func vocabulary(settings Settings, routes []web.Declaration) ([]string, error) {
	names := inspect.Names(routes)
	if settings.InspectAt == "" {
		return names, nil
	}
	inspector, err := inspect.Routes[effect.Unit, effect.Never](&inspect.Telemetry{}, settings.InspectAt)
	if err != nil {
		return nil, err
	}
	return append(append(names, web.PhaseNames()...), inspect.Names(web.DeclarationsOf(inspector...))...), nil
}

// Deliver sends traces for as long as scope lives, and what is left when it
// closes; it does nothing when no traces are exported.
func Deliver[R, E any](watch *Watch, scope effect.Scope) effect.Effect[R, E, effect.Unit] {
	if watch.exporter == nil {
		return effect.Succeed[R, E](effect.Unit{})
	}
	return effect.WidenError[E](otlp.Deliver[R](scope, watch.exporter))
}
