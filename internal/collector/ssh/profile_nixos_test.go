package ssh

import "testing"

func TestNixosMatch(t *testing.T) {
	if !(nixosProfile{}).Match(osFacts{ID: "nixos"}) {
		t.Error("should match nixos")
	}
	if (nixosProfile{}).Match(osFacts{ID: "debian"}) {
		t.Error("should not match debian")
	}
	// generic is the fallback for non-special OSes.
	if selectProfile(osFacts{ID: "debian"}).Name() != "generic" {
		t.Error("debian should fall back to generic")
	}
	if selectProfile(osFacts{ID: "nixos"}).Name() != "nixos" {
		t.Error("nixos should select the nixos profile")
	}
}

func TestParseNixStorePackages(t *testing.T) {
	out := `/nix/store/abcdefghijklmnopqrstuvwxyz012345-nginx-1.26.2
/nix/store/abcdefghijklmnopqrstuvwxyz012345-openssh-9.8p1
/nix/store/abcdefghijklmnopqrstuvwxyz012345-ca-certificates-bundle-20240705
/nix/store/abcdefghijklmnopqrstuvwxyz012345-hwdata
/nix/store/abcdefghijklmnopqrstuvwxyz012345-curl-8.20.0-bin
/nix/store/abcdefghijklmnopqrstuvwxyz012345-curl-8.20.0-man
/nix/store/abcdefghijklmnopqrstuvwxyz012345-less-692-man

not-a-store-path`
	sw := parseNixStorePackages(out)
	byName := map[string]string{}
	for _, s := range sw {
		byName[s.Name] = s.Version
	}
	if byName["nginx"] != "1.26.2" {
		t.Errorf("nginx version = %q", byName["nginx"])
	}
	if byName["openssh"] != "9.8p1" {
		t.Errorf("openssh version = %q", byName["openssh"])
	}
	// name with internal dashes, version preserved
	if byName["ca-certificates-bundle"] != "20240705" {
		t.Errorf("ca-certificates-bundle version = %q", byName["ca-certificates-bundle"])
	}
	// output suffix stripped (-bin / -man) and the two curl outputs deduped
	if byName["curl"] != "8.20.0" {
		t.Errorf("curl version = %q (output suffix should be stripped)", byName["curl"])
	}
	if byName["less"] != "692" {
		t.Errorf("less version = %q", byName["less"])
	}
	// no version → name only, no crash
	if _, ok := byName["hwdata"]; !ok {
		t.Error("hwdata (versionless) should be present")
	}
	// nginx, openssh, ca-certificates-bundle, hwdata, curl (deduped), less = 6
	if len(sw) != 6 {
		t.Fatalf("got %d packages, want 6: %+v", len(sw), sw)
	}
}
