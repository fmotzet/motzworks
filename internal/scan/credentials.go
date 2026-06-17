package scan

import (
	"context"
	"log/slog"
	"net/netip"

	"github.com/stock3/motzworks/internal/collector"
	"github.com/stock3/motzworks/internal/collector/snmp"
	"github.com/stock3/motzworks/internal/discovery"
	"github.com/stock3/motzworks/internal/store"
	"github.com/stock3/motzworks/internal/vault"
)

// DecryptCredentials unseals stored credentials into collector credentials.
// Credentials that fail to unseal are skipped (logged by the caller via count).
func DecryptCredentials(stored []store.StoredCredential, v *vault.Vault) []collector.Credential {
	var creds []collector.Credential
	for _, sc := range stored {
		secret := ""
		if sc.SecretSealed != "" {
			b, err := v.Open(sc.SecretSealed)
			if err != nil {
				continue
			}
			secret = string(b)
		}
		creds = append(creds, collector.Credential{
			ID:       sc.ID,
			Kind:     sc.Kind,
			Username: sc.Username,
			Secret:   secret,
			Extra:    sc.Extra,
		})
	}
	return creds
}

// SNMPProbe returns a discovery probe hook for the first SNMP credential in the
// set, or nil if none is present. It lets scheduled/ad-hoc scans find
// network-only gear, mirroring the CLI behavior.
func SNMPProbe(creds []collector.Credential, concurrency int, log *slog.Logger) func(context.Context, []netip.Addr) []discovery.Host {
	for _, c := range creds {
		if c.Kind == "snmp-v2c" || c.Kind == "snmp-v3" {
			cr := c
			sc := snmp.New(log)
			return func(ctx context.Context, addrs []netip.Addr) []discovery.Host {
				return sc.Probe(ctx, addrs, cr, concurrency)
			}
		}
	}
	return nil
}
