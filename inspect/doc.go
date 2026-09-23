// Package inspect serves a program's own telemetry, and the surface it serves.
//
// effect-golang-observe reads a runtime's events; this puts the reading where
// a person can see it. Two things follow from being a web module rather than a
// runtime one, and both are the reason it exists separately.
//
// It knows what is being served. A web.Routes carries its declarations -- the
// same values that dispatch a request and project into an OpenAPI document --
// so the inspector can show the surface beside the traffic through it, which
// no runtime-level tool can.
//
// And it can name a request. A runtime brackets what a program tells it to
// bracket, so a handler that opens no span of its own contributes nothing to a
// trace. Tracer is the middleware that makes every request a span named for
// its route, annotated with the method and the status it answered with.
//
// The inspector is itself an effect-golang-web program: a mountable Routes
// with a described JSON snapshot, its own OpenAPI contract, and a page. That
// is not a flourish -- a tool for inspecting this stack that was not built on
// it would be a tool nobody had tried the stack with.
package inspect
