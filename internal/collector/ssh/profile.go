package ssh

import (
	"strings"

	"github.com/stock3/motzworks/internal/model"
)

// runner runs a shell command on the target and returns its combined output.
type runner func(cmd string) string

// osFacts summarizes a host's identity, used to select an OS profile.
type osFacts struct {
	ID      string // os-release ID, lowercased: nixos, debian, ubuntu, alpine, rhel, ...
	Name    string
	Version string
	Kernel  string
	Arch    string
	Uname   string // uname -s: Linux, FreeBSD, Darwin
}

// Profile encapsulates OS-specific collection quirks. When an OS needs special
// handling, add a profile_<os>.go implementing this and register it in
// `profiles`. The generic profile covers mainstream Linux by autodetecting the
// package manager, so a new file is only needed for genuine edge cases.
//
// The interface intentionally starts small. A profile can opt into overriding
// other facets (users, hardware, …) by implementing an optional interface that
// the collector type-asserts for — no need to touch every profile.
type Profile interface {
	Name() string
	Match(osFacts) bool
	Software(run runner) []model.Software
}

// profiles are tried in order; the first Match wins, otherwise genericProfile.
var profiles = []Profile{
	nixosProfile{},
}

// selectProfile picks the handler for a host.
func selectProfile(f osFacts) Profile {
	for _, p := range profiles {
		if p.Match(f) {
			return p
		}
	}
	return genericProfile{}
}

// typeFor maps a host to a device type.
func typeFor(f osFacts) model.DeviceType {
	if familyFor(f) == "macos" {
		return model.TypeMac
	}
	return model.TypeLinux
}

// familyFor maps uname to an OS family.
func familyFor(f osFacts) string {
	switch {
	case strings.Contains(f.Uname, "FreeBSD"):
		return "freebsd"
	case strings.Contains(f.Uname, "Darwin"):
		return "macos"
	default:
		return "linux"
	}
}

// genericProfile handles mainstream Linux by trying dpkg, then rpm, then apk —
// the first that returns packages wins.
type genericProfile struct{}

func (genericProfile) Name() string       { return "generic" }
func (genericProfile) Match(osFacts) bool { return true }

func (genericProfile) Software(run runner) []model.Software {
	if out := run(`command -v dpkg-query >/dev/null 2>&1 && dpkg-query -W -f='${Package}\t${Version}\n' 2>/dev/null`); out != "" {
		if sw := parseTabbed(out); len(sw) > 0 {
			return sw
		}
	}
	if out := run(`command -v rpm >/dev/null 2>&1 && rpm -qa --qf '%{NAME}\t%{VERSION}-%{RELEASE}\n' 2>/dev/null`); out != "" {
		if sw := parseTabbed(out); len(sw) > 0 {
			return sw
		}
	}
	if out := run(`command -v apk >/dev/null 2>&1 && apk info -v 2>/dev/null`); out != "" {
		if sw := parseApk(out); len(sw) > 0 {
			return sw
		}
	}
	return nil
}
