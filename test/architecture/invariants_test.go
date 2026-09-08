package architecture

// The claims about this module's shape, rather than its behaviour.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// One package, which is the honest shape: an inspector is one thing. It reads
// what effect-golang-observe produces and serves it on effect-golang-web, and
// splitting that into layers would be layers with one member each.
var mayImport = map[string][]string{
	"inspect": {},
}

func TestPackagesDependOnNothingHereButThemselves(t *testing.T) {
	for pkg, allowed := range mayImport {
		permitted := map[string]bool{}
		for _, held := range allowed {
			permitted[held] = true
		}
		for _, source := range sourcesIn(t, pkg) {
			for _, line := range strings.Split(readSource(t, source), "\n") {
				held, found := ownImport(line)
				if !found || held == pkg || permitted[held] {
					continue
				}
				t.Errorf("%s imports %s, which it may not", display(t, source), held)
			}
		}
	}
}

// ownImport is an import of this module, as a package path within it. Only an
// import line counts: a doc comment naming a sibling package is prose.
func ownImport(line string) (string, bool) {
	if !imported(line) {
		return "", false
	}
	const prefix = `"github.com/mbauer83/effect-golang-observe-web/`
	at := strings.Index(line, prefix)
	if at < 0 {
		return "", false
	}
	rest := line[at+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// imported says the line is an import and not prose that happens to name a
// package. An import line is a quoted path and nothing else, optionally behind
// an alias or the import keyword.
func imported(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasSuffix(trimmed, `"`) {
		return false
	}
	trimmed = strings.TrimPrefix(trimmed, "import ")
	if strings.HasPrefix(trimmed, `"`) {
		return true
	}
	alias, rest, split := strings.Cut(trimmed, " ")
	return split && strings.HasPrefix(rest, `"`) && !strings.Contains(alias, `"`)
}

// TestNothingHereCarriesAThirdPartyDependency is the claim a debugging tool
// most needs to be able to make.
//
// A tool is installed in the program it watches, so every dependency it brings
// is a dependency that program now has -- and a version conflict between an
// inspector and the thing it inspects is the worst possible time to have one.
// Everything here is this project and the standard library. The page is one
// embedded document with no script, font or stylesheet from anywhere: a tool
// that only works where the internet is reachable is not a debugging tool.
func TestNothingHereCarriesAThirdPartyDependency(t *testing.T) {
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		relative := display(t, path)
		for _, line := range strings.Split(readSource(t, path), "\n") {
			if third, found := thirdPartyImport(line); found {
				t.Errorf("%s imports %q, and this module carries no dependencies",
					relative, third)
			}
		}
		return nil
	}
	if err := filepath.WalkDir(moduleRoot(t), walk); err != nil {
		t.Fatal(err)
	}
}

// thirdPartyImport reports an import that is neither the standard library nor
// this project. A standard-library path has no dot before its first slash.
func thirdPartyImport(line string) (string, bool) {
	if !imported(line) {
		return "", false
	}
	trimmed := strings.TrimSpace(line)
	start := strings.Index(trimmed, `"`)
	path := strings.Trim(trimmed[start:], `"`)
	host, _, hasSlash := strings.Cut(path, "/")
	if !hasSlash || !strings.Contains(host, ".") {
		return "", false
	}
	if strings.HasPrefix(path, "github.com/mbauer83/") {
		return "", false
	}
	return path, true
}

// The module root is for project metadata and documentation. Every package is
// a directory, for the reason the runtime's is: a root full of source files is
// not a layout.
func TestModuleRootHoldsNoSource(t *testing.T) {
	entries, err := os.ReadDir(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			t.Errorf("%s is in the module root", entry.Name())
		}
	}
}

// A file that grows past these has stopped being about one thing. The soft
// limit is a review prompt and the hard one is a failure, so the check is a
// test rather than something a growing file can do by drifting past a review.
const (
	soft = 250
	hard = 350
)

func TestSourceFilesStayWithinTheirLineLimits(t *testing.T) {
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		lines := strings.Count(readSource(t, path), "\n")
		relative := display(t, path)
		switch {
		case lines > hard:
			t.Errorf("%s has %d lines, past the hard limit", relative, lines)
		case lines > soft:
			t.Errorf("%s has %d lines, past the soft limit; split it by domain role",
				relative, lines)
		}
		return nil
	}
	if err := filepath.WalkDir(moduleRoot(t), walk); err != nil {
		t.Fatal(err)
	}
}

// The page and its assets are served to people, so they are checked in and
// embedded rather than generated: a reviewer reads the same bytes a browser
// gets.
//
// Nothing is fetched from off this process. The rule is about *fetches* and
// not about the string "http": an SVG namespace is a URL and not a request,
// and the first version of this test failed on one.
func TestThePageFetchesNothingFromOffThisProcess(t *testing.T) {
	page := string(readSource(t, filepath.Join(moduleRoot(t), "inspect", "page.html")))
	for _, reaching := range []string{
		`src="http`, `src='http`, `href="http`, `href='http`,
		"url(http", "//cdn", "//unpkg", `from "http`, "import(\"http",
	} {
		if strings.Contains(page, reaching) {
			t.Errorf("the page fetches from off this process: %q", reaching)
		}
	}
}

// The vendored chart library is third-party code, so its licence travels with
// it. A dependency nobody recorded is the one that becomes a problem.
func TestTheVendoredLibraryCarriesItsLicence(t *testing.T) {
	assets := filepath.Join(moduleRoot(t), "inspect", "assets")
	licence := string(readSource(t, filepath.Join(assets, "uplot.LICENSE")))
	if !strings.Contains(licence, "MIT") {
		t.Errorf("the vendored library's licence is not the one recorded: %q",
			first(licence))
	}
	// And the code says which version it is, so an upgrade is a reviewable
	// change rather than a silent one.
	script := string(readSource(t, filepath.Join(assets, "uplot.min.js")))
	if !strings.Contains(first(script), "uPlot") || !strings.Contains(first(script), "v1.") {
		t.Errorf("the vendored library does not say what it is: %q", first(script))
	}
}

func first(content string) string {
	line, _, _ := strings.Cut(content, "\n")
	return line
}

// A document nobody can reach is a document nobody reads.
func TestEveryPublicDocumentIsLinkedFromTheReadme(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(moduleRoot(t), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		relative := filepath.ToSlash(display(t, path))
		if !strings.Contains(string(readme), relative) {
			t.Errorf("%s is not linked from README.md", relative)
		}
		return nil
	}
	if err := filepath.WalkDir(filepath.Join(moduleRoot(t), "docs"), walk); err != nil {
		t.Fatal(err)
	}
}
