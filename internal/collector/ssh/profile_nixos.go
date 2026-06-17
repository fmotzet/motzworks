package ssh

import (
	"regexp"
	"strings"

	"github.com/stock3/motzworks/internal/model"
)

// nixosProfile collects software on NixOS, which has no dpkg/rpm/apk. The set of
// system packages is the direct references of /run/current-system/sw (i.e. what
// `environment.systemPackages` produced), each a store path of the form
// /nix/store/<hash>-<name>-<version>.
type nixosProfile struct{}

func (nixosProfile) Name() string { return "nixos" }

func (nixosProfile) Match(f osFacts) bool { return f.ID == "nixos" }

func (nixosProfile) Software(run runner) []model.Software {
	// Absolute path: a non-interactive SSH session may not have nix-store on PATH.
	out := run(`/run/current-system/sw/bin/nix-store -q --references /run/current-system/sw 2>/dev/null`)
	return parseNixStorePackages(out)
}

var (
	// store path basename: 32-char hash, a dash, then "<name>-<version>".
	nixHashRe = regexp.MustCompile(`^[a-z0-9]{32}-(.+)$`)
	// split a trailing version (starts with a digit) off the name.
	nixNameVerRe = regexp.MustCompile(`^(.+?)-([0-9][a-zA-Z0-9.+_~-]*)$`)
	// trailing Nix output name (curl-8.20.0-bin, less-692-man, …) — stripped so
	// a package's split outputs collapse into one entry.
	nixOutputRe = regexp.MustCompile(`-(bin|man|dev|devdoc|doc|info|lib|out|static|debug)$`)
)

// parseNixStorePackages turns `nix-store --references` output into software.
func parseNixStorePackages(out string) []model.Software {
	seen := map[string]bool{}
	var sw []model.Software
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		base := line[strings.LastIndexByte(line, '/')+1:]
		m := nixHashRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		name, version := m[1], ""
		if nv := nixNameVerRe.FindStringSubmatch(m[1]); nv != nil {
			name, version = nv[1], nixOutputRe.ReplaceAllString(nv[2], "")
		}
		key := name + "\x00" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		sw = append(sw, model.Software{Name: name, Version: version})
	}
	return sw
}
