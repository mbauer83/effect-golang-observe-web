package observability

import "github.com/mbauer83/effect-golang/effect/config"

// Config describes Settings in the variables OpenTelemetry defines,
// OTEL_SERVICE_NAME and OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, with METRICS_AT,
// METRICS_NAMESPACE and INSPECT_AT beside them. A program with its own
// spelling fills Settings itself.
//
//	config.Setting(observability.Config, func(s *Settings, watch observability.Settings) { s.Watch = watch })
var Config = config.Struct(
	config.Setting(config.Text("otel_service_name").WithDefault("").
		WithDescription("the service named on every span and series"),
		func(settings *Settings, service string) { settings.Service = service }),
	config.Setting(config.Text("otel_exporter_otlp_traces_endpoint").WithDefault("").
		WithDescription("the collector's OTLP/HTTP traces endpoint; no traces are exported when empty"),
		func(settings *Settings, endpoint string) { settings.TracesTo = endpoint }),
	config.Setting(config.Text("metrics_at").WithDefault("").
		WithDescription("the path Prometheus scrapes; not served when empty"),
		func(settings *Settings, path string) { settings.MetricsAt = path }),
	config.Setting(config.Text("metrics_namespace").WithDefault("runtime").
		WithDescription("what every metric name begins with"),
		func(settings *Settings, namespace string) { settings.Namespace = namespace }),
	config.Setting(config.Text("inspect_at").WithDefault("").
		WithDescription("the path of the runtime inspector; not mounted when empty"),
		func(settings *Settings, path string) { settings.InspectAt = path }),
)
