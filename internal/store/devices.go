package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stock3/motzworks/internal/model"
)

// ChangeEvent is a scalar field that differed from the stored device.
type ChangeEvent struct {
	Field string
	Old   string
	New   string
}

// existing holds the stored scalar values used to diff against an incoming
// device when computing change events.
type existing struct {
	Type      string
	Hostname  string
	PrimaryIP string
	Serial    string
	OSName    string
	OSVersion string
}

// UpsertDevice resolves the device's identity, inserts or updates it together
// with its child records, records scalar field changes, and returns the device
// id and the list of changes. The whole operation runs in one transaction.
func (s *Store) UpsertDevice(ctx context.Context, d model.Device, scanRunID string) (string, []ChangeEvent, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(ctx)

	id, ex, err := resolveDevice(ctx, tx, d)
	if err != nil {
		return "", nil, err
	}

	now := time.Now()
	var changes []ChangeEvent

	if id == "" {
		if err := tx.QueryRow(ctx, `
			INSERT INTO device
			  (device_type, hostname, primary_ip, serial, asset_tag, ad_guid, source, first_seen, last_seen)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
			RETURNING id`,
			string(d.Type), nullStr(d.Hostname), nullIP(d.PrimaryIP), nullStr(d.Serial),
			nullStr(d.AssetTag), nullStr(d.ADGuid), nullStr(d.Source), now,
		).Scan(&id); err != nil {
			return "", nil, err
		}
	} else {
		changes = diffDevice(ex, d)
		// COALESCE keeps prior values when the new scan reports nothing for a
		// field, so a shallow scan never erases data from a deeper one.
		if _, err := tx.Exec(ctx, `
			UPDATE device SET
			  device_type = CASE WHEN $2 <> 'unknown' THEN $2 ELSE device_type END,
			  hostname    = COALESCE($3, hostname),
			  primary_ip  = COALESCE($4, primary_ip),
			  serial      = COALESCE($5, serial),
			  asset_tag   = COALESCE($6, asset_tag),
			  ad_guid     = COALESCE($7, ad_guid),
			  source      = COALESCE($8, source),
			  last_seen   = $9,
			  updated_at  = $9
			WHERE id = $1`,
			id, string(d.Type), nullStr(d.Hostname), nullIP(d.PrimaryIP), nullStr(d.Serial),
			nullStr(d.AssetTag), nullStr(d.ADGuid), nullStr(d.Source), now,
		); err != nil {
			return "", nil, err
		}
	}

	if err := replaceChildren(ctx, tx, id, d); err != nil {
		return "", nil, err
	}

	for _, ch := range changes {
		if _, err := tx.Exec(ctx, `
			INSERT INTO change_event (device_id, scan_run_id, field, old_value, new_value)
			VALUES ($1, $2, $3, $4, $5)`,
			id, nullStr(scanRunID), ch.Field, ch.Old, ch.New,
		); err != nil {
			return "", nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return id, changes, nil
}

// resolveDevice finds an existing device by identity keys in priority order:
// serial, AD GUID, any interface MAC, hostname, then primary IP. It returns the
// matched id (empty if new) and a snapshot of stored scalars for diffing.
func resolveDevice(ctx context.Context, tx pgx.Tx, d model.Device) (string, existing, error) {
	var id string

	queryID := func(q string, args ...any) (bool, error) {
		err := tx.QueryRow(ctx, q, args...).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return id != "", nil
	}

	type attempt struct {
		when bool
		q    string
		args []any
	}
	macs := macsOf(d)
	attempts := []attempt{
		{d.Serial != "", `SELECT id FROM device WHERE serial = $1`, []any{d.Serial}},
		{d.ADGuid != "", `SELECT id FROM device WHERE ad_guid = $1`, []any{d.ADGuid}},
		{len(macs) > 0, `SELECT device_id FROM interface WHERE mac = ANY($1::macaddr[]) LIMIT 1`, []any{macs}},
		{d.Hostname != "", `SELECT id FROM device WHERE lower(hostname) = lower($1) LIMIT 1`, []any{d.Hostname}},
		{d.PrimaryIP.IsValid(), `SELECT id FROM device WHERE primary_ip = $1 LIMIT 1`, []any{ipStr(d.PrimaryIP)}},
	}
	for _, a := range attempts {
		if !a.when {
			continue
		}
		found, err := queryID(a.q, a.args...)
		if err != nil {
			return "", existing{}, err
		}
		if found {
			break
		}
	}

	if id == "" {
		return "", existing{}, nil
	}

	var ex existing
	if err := tx.QueryRow(ctx, `
		SELECT device_type, COALESCE(hostname,''), COALESCE(host(primary_ip),''), COALESCE(serial,'')
		FROM device WHERE id = $1`, id,
	).Scan(&ex.Type, &ex.Hostname, &ex.PrimaryIP, &ex.Serial); err != nil {
		return "", existing{}, err
	}
	// OS facts live in a side table; absence is fine.
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(name,''), COALESCE(version,'') FROM os_info WHERE device_id = $1`, id,
	).Scan(&ex.OSName, &ex.OSVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", existing{}, err
	}
	return id, ex, nil
}

// diffDevice compares stored scalars with an incoming device. Empty incoming
// values are ignored so a partial scan does not register false "removals".
func diffDevice(ex existing, d model.Device) []ChangeEvent {
	var ch []ChangeEvent
	add := func(field, old, new string) {
		if new != "" && new != old {
			ch = append(ch, ChangeEvent{Field: field, Old: old, New: new})
		}
	}
	add("hostname", ex.Hostname, d.Hostname)
	if d.PrimaryIP.IsValid() {
		add("primary_ip", ex.PrimaryIP, d.PrimaryIP.String())
	}
	add("serial", ex.Serial, d.Serial)
	if d.Type != "" && d.Type != model.TypeUnknown {
		add("device_type", ex.Type, string(d.Type))
	}
	if d.OS != nil {
		add("os_name", ex.OSName, d.OS.Name)
		add("os_version", ex.OSVersion, d.OS.Version)
	}
	return ch
}

// replaceChildren rewrites the multi-row child tables and upserts the single-row
// os/hardware tables. Multi-row tables are cleared then reinserted so removed
// software/interfaces/users disappear on the next scan.
func replaceChildren(ctx context.Context, tx pgx.Tx, id string, d model.Device) error {
	for _, table := range []string{"interface", "software", "user_account"} {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE device_id = $1", id); err != nil {
			return err
		}
	}

	for _, ifc := range d.Interfaces {
		if _, err := tx.Exec(ctx, `
			INSERT INTO interface (device_id, name, mac, ip, speed_mbps, vlan)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			id, nullStr(ifc.Name), nullStr(ifc.MAC), nullIP(ifc.IP),
			nullInt64(ifc.SpeedMbps), nullInt(ifc.VLAN),
		); err != nil {
			return err
		}
	}

	if d.OS != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO os_info (device_id, family, name, version, build, arch)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (device_id) DO UPDATE SET
			  family = EXCLUDED.family, name = EXCLUDED.name, version = EXCLUDED.version,
			  build = EXCLUDED.build, arch = EXCLUDED.arch`,
			id, nullStr(d.OS.Family), nullStr(d.OS.Name), nullStr(d.OS.Version),
			nullStr(d.OS.Build), nullStr(d.OS.Arch),
		); err != nil {
			return err
		}
	}

	if d.Hardware != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO hardware (device_id, vendor, model, serial, cpu, cpu_cores, ram_bytes)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (device_id) DO UPDATE SET
			  vendor = EXCLUDED.vendor, model = EXCLUDED.model, serial = EXCLUDED.serial,
			  cpu = EXCLUDED.cpu, cpu_cores = EXCLUDED.cpu_cores, ram_bytes = EXCLUDED.ram_bytes`,
			id, nullStr(d.Hardware.Vendor), nullStr(d.Hardware.Model), nullStr(d.Hardware.Serial),
			nullStr(d.Hardware.CPU), nullInt(d.Hardware.CPUCores), nullInt64(d.Hardware.RAMBytes),
		); err != nil {
			return err
		}
	}

	for _, sw := range d.Software {
		if _, err := tx.Exec(ctx, `
			INSERT INTO software (device_id, name, version, vendor, install_date)
			VALUES ($1,$2,$3,$4,$5)`,
			id, sw.Name, nullStr(sw.Version), nullStr(sw.Vendor), nullDate(sw.InstallDate),
		); err != nil {
			return err
		}
	}

	for _, u := range d.Users {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_account (device_id, username, full_name, last_logon, is_local)
			VALUES ($1,$2,$3,$4,$5)`,
			id, u.Username, nullStr(u.FullName), u.LastLogon, u.IsLocal,
		); err != nil {
			return err
		}
	}
	return nil
}

// macsOf returns the non-empty interface MACs of a device.
func macsOf(d model.Device) []string {
	var macs []string
	for _, ifc := range d.Interfaces {
		if ifc.MAC != "" {
			macs = append(macs, ifc.MAC)
		}
	}
	return macs
}
