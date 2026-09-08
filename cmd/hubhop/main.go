// Command hubhop helps author aircraft profiles.
//
// It has two modes:
//
//	hubhop search -sim msfs -aircraft "PMDG.*B737" -match autopilot
//	    Queries the MobiFlight HubHop community database
//	    (https://hubhop.mobiflight.com) for the variables an add-on publishes
//	    and prints the readable ones. Nothing is stored in the repository —
//	    the profiles under internal/profiles/builtin are hand-written from
//	    what this prints, with a stock simulation variable as the fallback.
//
//	hubhop scan "C:\...\Community"
//	    Walks an MSFS Community folder and reports, per package, the aircraft
//	    titles and ICAO types from aircraft.cfg plus the local variables the
//	    model behaviour files reference. Use it to find the variables for an
//	    add-on HubHop does not cover, and to see the exact titles a profile
//	    selector has to match.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const hubHopAPI = "https://hubhop-api-mgtm.azure-api.net/api/v1/%s/presets"

// preset is one HubHop entry. Only the fields a profile author needs are kept.
type preset struct {
	Vendor      string `json:"vendor"`
	Aircraft    string `json:"aircraft"`
	System      string `json:"system"`
	Label       string `json:"label"`
	Code        string `json:"code"`
	PresetType  string `json:"presetType"`
	Description string `json:"description"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "search":
		search(os.Args[2:])
	case "scan":
		scan(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage:
  hubhop search [-sim msfs|xplane] [-aircraft REGEX] [-match REGEX] [-all]
  hubhop scan   DIRECTORY [-lvars N]   (the directory may come before or after the flags)
`)
	os.Exit(2)
}

func search(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	sim := fs.String("sim", "msfs", "simulator database: msfs or xplane")
	aircraft := fs.String("aircraft", "", "regular expression matched against \"vendor aircraft\"")
	match := fs.String("match", "", "regular expression matched against the label, code and description")
	all := fs.Bool("all", false, "include input presets; by default only readable ones are listed")
	fs.Parse(args)

	acRe, err := regexp.Compile("(?i)" + *aircraft)
	exitOn(err)
	kwRe, err := regexp.Compile("(?i)" + *match)
	exitOn(err)

	presets, err := fetch(*sim)
	exitOn(err)

	seen := map[string]bool{}
	count := 0
	for _, p := range presets {
		if !*all && !strings.Contains(p.PresetType, "Output") {
			continue
		}
		if !acRe.MatchString(p.Vendor + " " + p.Aircraft) {
			continue
		}
		if !kwRe.MatchString(p.Label + " " + p.Code + " " + p.Description) {
			continue
		}
		key := p.Aircraft + "|" + p.Code
		if seen[key] {
			continue
		}
		seen[key] = true
		count++
		fmt.Printf("%-28s %-14s %s\n", trunc(p.Vendor+" "+p.Aircraft, 28), trunc(p.System, 14), p.Code)
		if d := strings.TrimSpace(strings.ReplaceAll(p.Description, "\n", " ")); d != "" {
			fmt.Printf("%-44s %s\n", "", trunc(d, 100))
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d variable(s) from %d preset(s)\n", count, len(presets))
}

func fetch(sim string) ([]preset, error) {
	switch sim {
	case "msfs", "msfs2020", "msfs2024":
		sim = "msfs2020"
	case "xplane", "xplane11", "xplane12":
		sim = "xplane"
	default:
		return nil, fmt.Errorf("unknown simulator %q", sim)
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(fmt.Sprintf(hubHopAPI, sim))
	if err != nil {
		return nil, fmt.Errorf("fetch HubHop: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HubHop returned %s", resp.Status)
	}

	var presets []preset
	if err := json.NewDecoder(resp.Body).Decode(&presets); err != nil {
		return nil, fmt.Errorf("decode HubHop response: %w", err)
	}
	return presets, nil
}

var (
	lvarRe  = regexp.MustCompile(`L:([A-Za-z0-9_.]{3,})`)
	varTag  = regexp.MustCompile(`<VAR_NAME[^>]*>([^<]+)<`)
	titleRe = regexp.MustCompile(`(?im)^\s*title\s*=\s*"?([^"\r\n]+)"?`)
	icaoRe  = regexp.MustCompile(`(?im)^\s*icao_type_designator\s*=\s*"?([^"\r\n]+)"?`)
)

// flagsFirst moves the directory to the end so the flag package still sees the
// options when the directory is given first, which is the natural way to type it.
func flagsFirst(args []string) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			flags = append(flags, args[i])
			// A value-taking flag written as "-lvars 6" carries its value next.
			if !strings.Contains(args[i], "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		rest = append(rest, args[i])
	}
	return append(flags, rest...)
}

// scanned collects what one add-on package advertises.
type scanned struct {
	titles map[string]bool
	icao   map[string]bool
	lvars  map[string]int
}

func scan(args []string) {
	fset := flag.NewFlagSet("scan", flag.ExitOnError)
	top := fset.Int("lvars", 25, "how many of the most-referenced local variables to print per package")
	fset.Parse(flagsFirst(args))
	if fset.NArg() != 1 {
		usage()
	}
	root := fset.Arg(0)

	// One entry per package: the directory directly under the scanned root.
	packages := map[string]*scanned{}
	pkgOf := func(path string) string {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "."
		}
		if i := strings.IndexRune(rel, filepath.Separator); i > 0 {
			return rel[:i]
		}
		return rel
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping %s: %v\n", path, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		name := strings.ToLower(d.Name())
		ext := filepath.Ext(name)
		isCfg := name == "aircraft.cfg"
		isBehaviour := ext == ".xml" || ext == ".html" || ext == ".js"
		if !isCfg && !isBehaviour {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > 32<<20 {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		pkg := pkgOf(path)
		s := packages[pkg]
		if s == nil {
			s = &scanned{titles: map[string]bool{}, icao: map[string]bool{}, lvars: map[string]int{}}
			packages[pkg] = s
		}

		if isCfg {
			for _, m := range titleRe.FindAllStringSubmatch(string(body), -1) {
				s.titles[strings.TrimSpace(m[1])] = true
			}
			for _, m := range icaoRe.FindAllStringSubmatch(string(body), -1) {
				s.icao[strings.TrimSpace(m[1])] = true
			}
			return nil
		}
		for _, m := range lvarRe.FindAllStringSubmatch(string(body), -1) {
			s.lvars[m[1]]++
		}
		for _, m := range varTag.FindAllStringSubmatch(string(body), -1) {
			s.lvars[strings.TrimSpace(m[1])]++
		}
		return nil
	})
	exitOn(err)

	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for _, name := range names {
		s := packages[name]
		if len(s.titles) == 0 && len(s.lvars) == 0 {
			continue
		}
		fmt.Fprintf(out, "\n=== %s\n", name)
		if titles := sortedKeys(s.titles); len(titles) > 0 {
			fmt.Fprintf(out, "  titles (%d): %s\n", len(titles), strings.Join(clip(titles, 6), " | "))
		}
		if icao := sortedKeys(s.icao); len(icao) > 0 {
			fmt.Fprintf(out, "  icao type:   %s\n", strings.Join(icao, ", "))
		}
		if len(s.lvars) > 0 {
			fmt.Fprintf(out, "  local variables (%d, most referenced first):\n", len(s.lvars))
			for _, v := range mostUsed(s.lvars, *top) {
				fmt.Fprintf(out, "    %s\n", v)
			}
		}
	}
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func clip(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return append(values[:n:n], fmt.Sprintf("… %d more", len(values)-n))
}

func mostUsed(counts map[string]int, n int) []string {
	type kv struct {
		name string
		hits int
	}
	all := make([]kv, 0, len(counts))
	for k, v := range counts {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].hits != all[j].hits {
			return all[i].hits > all[j].hits
		}
		return all[i].name < all[j].name
	})
	if len(all) > n {
		all = all[:n]
	}
	out := make([]string, 0, len(all))
	for _, e := range all {
		out = append(out, fmt.Sprintf("%-48s %d", e.name, e.hits))
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func exitOn(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "hubhop:", err)
		os.Exit(1)
	}
}
