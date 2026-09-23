package inspect

// The document the inspector serves, and the assets it serves beside it.

import (
	"embed"
)

// document is the page and its assets, checked in and embedded.
//
// Nothing is fetched from anywhere at run time: no CDN, no font, no
// stylesheet from a third party's domain. A tool that only works where the
// internet is reachable is not a debugging tool, and a page that pulled a
// script from somewhere would be a page nobody could serve from a machine
// behind a firewall.
//
// uPlot is vendored rather than reimplemented. Axes, a cursor that reads
// values off the series under the pointer, and a legend are a chart library's
// job, and fifty kilobytes of MIT-licensed code with no dependencies of its
// own is a smaller thing to carry than a hand-rolled approximation of them.
// Its licence is checked in beside it.
//
//go:embed page.html assets
var document embed.FS

// Page is the inspector's document.
//
// Read once at start rather than per request: these are checked-in files and
// cannot change while the program runs, so re-reading them would be a syscall
// per refresh for bytes that are already correct.
func Page() []byte {
	return static.page
}

// Script and Stylesheet are the vendored chart library, served beside the
// page. Public because a program mounting the inspector by hand needs to be
// able to serve them.
func Script() []byte {
	return static.script
}

func Stylesheet() []byte {
	return static.stylesheet
}

// Licence is the vendored library's licence, served beside it so the
// attribution travels with the code rather than only with the repository.
func Licence() []byte {
	return static.licence
}

var static = read()

type files struct {
	page       []byte
	script     []byte
	stylesheet []byte
	licence    []byte
}

func read() files {
	return files{
		page:       mustRead("page.html"),
		script:     mustRead("assets/uplot.min.js"),
		stylesheet: mustRead("assets/uplot.min.css"),
		licence:    mustRead("assets/uplot.LICENSE"),
	}
}

func mustRead(name string) []byte {
	content, err := document.ReadFile(name)
	if err != nil {
		// Unreachable: these are embedded at build time, so a missing one is a
		// build failure rather than a run-time outcome. Panicking says that,
		// where returning an error would ask every caller to handle something
		// that cannot happen.
		panic("inspect: the embedded file " + name + " is missing: " + err.Error())
	}
	return content
}
