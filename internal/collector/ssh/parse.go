package ssh

import (
	"net/netip"
	"strconv"
	"strings"

	"github.com/stock3/motzworks/internal/model"
)

// parseOSRelease extracts name, version and the lowercased ID from
// /etc/os-release content.
func parseOSRelease(content string) (name, version, id string) {
	kv := map[string]string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		kv[k] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	name = kv["PRETTY_NAME"]
	if name == "" {
		name = kv["NAME"]
	}
	version = kv["VERSION_ID"]
	id = strings.ToLower(kv["ID"])
	return name, version, id
}

// parseTabbed parses "name<TAB>version" lines (dpkg-query / rpm output).
func parseTabbed(out string) []model.Software {
	var sw []model.Software
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, ver, _ := strings.Cut(line, "\t")
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		sw = append(sw, model.Software{Name: name, Version: strings.TrimSpace(ver)})
	}
	return sw
}

// parseApk parses `apk info -v` lines of the form name-version-release, where
// the name itself may contain dashes.
func parseApk(out string) []model.Software {
	var sw []model.Software
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "-")
		if len(parts) >= 3 {
			name := strings.Join(parts[:len(parts)-2], "-")
			ver := parts[len(parts)-2] + "-" + parts[len(parts)-1]
			sw = append(sw, model.Software{Name: name, Version: ver})
		} else {
			sw = append(sw, model.Software{Name: line})
		}
	}
	return sw
}

// parseIPLink parses `ip -o link show`, returning interfaces with MAC addresses
// keyed by name (loopback excluded).
func parseIPLink(out string) []model.Interface {
	var ifaces []model.Interface
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "2: eth0@if13: <BROADCAST,...> mtu 1500 ... link/ether aa:bb:.. brd .."
		fields := strings.Fields(line)
		var name, mac string
		for i, f := range fields {
			if i == 1 {
				name = normalizeIfName(f)
			}
			if f == "link/ether" && i+1 < len(fields) {
				mac = fields[i+1]
			}
		}
		if name == "" || name == "lo" || mac == "" {
			continue
		}
		ifaces = append(ifaces, model.Interface{Name: name, MAC: mac})
	}
	return ifaces
}

// applyIPAddrs parses `ip -o addr show` and assigns the first IP found per
// interface name to the matching interface.
func applyIPAddrs(ifaces []model.Interface, out string) {
	byName := map[string]*model.Interface{}
	for i := range ifaces {
		byName[ifaces[i].Name] = &ifaces[i]
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		// "2: eth0    inet 10.0.0.5/24 brd ..."
		var name, cidr string
		for i, f := range fields {
			if i == 1 {
				name = normalizeIfName(f)
			}
			if (f == "inet" || f == "inet6") && i+1 < len(fields) {
				cidr = fields[i+1]
			}
		}
		ifc, ok := byName[name]
		if !ok || cidr == "" || ifc.IP.IsValid() {
			continue
		}
		if pfx, err := netip.ParsePrefix(cidr); err == nil {
			ifc.IP = pfx.Addr()
		}
	}
}

// normalizeIfName strips the trailing ":" and any "@peer" veth suffix from an
// interface token (e.g. "eth0@if13:" -> "eth0") so link and addr output match.
func normalizeIfName(f string) string {
	f = strings.TrimSuffix(f, ":")
	if i := strings.IndexByte(f, '@'); i >= 0 {
		f = f[:i]
	}
	return f
}

// maxUID excludes the conventional "nobody" account (65534) and other very high
// system UIDs from the reported user list.
const maxUID = 65000

// parsePasswd parses getent/passwd output. Accounts with uid 0 or minUID <= uid
// < maxUID are returned (filtering out most system accounts).
func parsePasswd(out string, minUID int) []model.UserAccount {
	var users []model.UserAccount
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, ":")
		if len(f) < 7 {
			continue
		}
		uid, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		if uid != 0 && (uid < minUID || uid >= maxUID) {
			continue
		}
		full := f[4]
		if i := strings.IndexByte(full, ','); i >= 0 {
			full = full[:i] // GECOS first field is the real name
		}
		users = append(users, model.UserAccount{
			Username: f[0],
			FullName: strings.TrimSpace(full),
			IsLocal:  true,
		})
	}
	return users
}

// parseMemTotalBytes extracts total RAM in bytes from /proc/meminfo.
func parseMemTotalBytes(out string) int64 {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				return kb * 1024 // value is in kB
			}
		}
	}
	return 0
}

// parseCPUInfo returns the CPU model name and logical core count.
func parseCPUInfo(out string) (modelName string, cores int) {
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		switch key {
		case "processor":
			cores++
		case "model name", "Model":
			if modelName == "" {
				modelName = strings.TrimSpace(val)
			}
		}
	}
	return modelName, cores
}
