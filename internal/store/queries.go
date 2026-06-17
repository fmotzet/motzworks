package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- DTOs (JSON-ready, returned to the API layer) ----

// DeviceListItem is a row in the device list.
type DeviceListItem struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Hostname  string    `json:"hostname"`
	PrimaryIP string    `json:"primary_ip"`
	OSName    string    `json:"os_name"`
	Source    string    `json:"source"`
	LastSeen  time.Time `json:"last_seen"`
}

// DeviceDetail is the full record for one device.
type DeviceDetail struct {
	DeviceListItem
	Serial     string        `json:"serial"`
	FirstSeen  time.Time     `json:"first_seen"`
	OS         *OSDTO        `json:"os"`
	Hardware   *HardwareDTO  `json:"hardware"`
	Interfaces []IfaceDTO    `json:"interfaces"`
	Software   []SoftwareDTO `json:"software"`
	Users      []UserDTO     `json:"users"`
}

type OSDTO struct {
	Family  string `json:"family"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Arch    string `json:"arch"`
}

type HardwareDTO struct {
	Vendor   string `json:"vendor"`
	Model    string `json:"model"`
	Serial   string `json:"serial"`
	CPU      string `json:"cpu"`
	CPUCores int    `json:"cpu_cores"`
	RAMBytes int64  `json:"ram_bytes"`
}

type IfaceDTO struct {
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	SpeedMbps int64  `json:"speed_mbps"`
}

type SoftwareDTO struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Vendor  string `json:"vendor"`
}

type UserDTO struct {
	Username  string     `json:"username"`
	FullName  string     `json:"full_name"`
	LastLogon *time.Time `json:"last_logon"`
	IsLocal   bool       `json:"is_local"`
}

// DeviceFilter parameterizes ListDevices.
type DeviceFilter struct {
	Query  string
	Type   string
	Limit  int
	Offset int
}

// argBuilder accumulates positional query args and yields $N placeholders.
type argBuilder struct{ args []any }

func (b *argBuilder) next(v any) string {
	b.args = append(b.args, v)
	return fmt.Sprintf("$%d", len(b.args))
}

// buildDeviceWhere builds the shared WHERE clause for device list/count.
func buildDeviceWhere(f DeviceFilter, b *argBuilder) string {
	clauses := []string{"1=1"}
	if q := strings.TrimSpace(f.Query); q != "" {
		p := b.next("%" + q + "%")
		clauses = append(clauses, fmt.Sprintf(
			"(d.hostname ILIKE %s OR host(d.primary_ip) ILIKE %s OR d.serial ILIKE %s)", p, p, p))
	}
	if f.Type != "" {
		clauses = append(clauses, "d.device_type = "+b.next(f.Type))
	}
	return strings.Join(clauses, " AND ")
}

// ListDevices returns a page of devices plus the total matching count.
func (s *Store) ListDevices(ctx context.Context, f DeviceFilter) ([]DeviceListItem, int, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 50
	}

	var cb argBuilder
	where := buildDeviceWhere(f, &cb)

	var total int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM device d WHERE "+where, cb.args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	var lb argBuilder
	where = buildDeviceWhere(f, &lb)
	limit := lb.next(f.Limit)
	offset := lb.next(f.Offset)
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.device_type, COALESCE(d.hostname,''), COALESCE(host(d.primary_ip),''),
		       COALESCE(o.name,''), COALESCE(d.source,''), d.last_seen
		FROM device d
		LEFT JOIN os_info o ON o.device_id = d.id
		WHERE `+where+`
		ORDER BY d.last_seen DESC
		LIMIT `+limit+` OFFSET `+offset, lb.args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []DeviceListItem
	for rows.Next() {
		var d DeviceListItem
		if err := rows.Scan(&d.ID, &d.Type, &d.Hostname, &d.PrimaryIP, &d.OSName, &d.Source, &d.LastSeen); err != nil {
			return nil, 0, err
		}
		items = append(items, d)
	}
	return items, total, rows.Err()
}

// GetDevice returns the full detail for one device, or (nil, nil) if not found.
func (s *Store) GetDevice(ctx context.Context, id string) (*DeviceDetail, error) {
	var d DeviceDetail
	err := s.pool.QueryRow(ctx, `
		SELECT id, device_type, COALESCE(hostname,''), COALESCE(host(primary_ip),''),
		       COALESCE(source,''), last_seen, first_seen, COALESCE(serial,'')
		FROM device WHERE id = $1`, id,
	).Scan(&d.ID, &d.Type, &d.Hostname, &d.PrimaryIP, &d.Source, &d.LastSeen, &d.FirstSeen, &d.Serial)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// OS (optional)
	var os OSDTO
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(family,''), COALESCE(name,''), COALESCE(version,''), COALESCE(build,''), COALESCE(arch,'')
		FROM os_info WHERE device_id = $1`, id,
	).Scan(&os.Family, &os.Name, &os.Version, &os.Build, &os.Arch)
	if err == nil {
		d.OS = &os
		d.OSName = os.Name
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	// Hardware (optional)
	var hw HardwareDTO
	err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(vendor,''), COALESCE(model,''), COALESCE(serial,''),
		       COALESCE(cpu,''), COALESCE(cpu_cores,0), COALESCE(ram_bytes,0)
		FROM hardware WHERE device_id = $1`, id,
	).Scan(&hw.Vendor, &hw.Model, &hw.Serial, &hw.CPU, &hw.CPUCores, &hw.RAMBytes)
	if err == nil {
		d.Hardware = &hw
	} else if err != pgx.ErrNoRows {
		return nil, err
	}

	if d.Interfaces, err = s.deviceInterfaces(ctx, id); err != nil {
		return nil, err
	}
	if d.Software, err = s.deviceSoftware(ctx, id); err != nil {
		return nil, err
	}
	if d.Users, err = s.deviceUsers(ctx, id); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) deviceInterfaces(ctx context.Context, id string) ([]IfaceDTO, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(name,''), COALESCE(mac::text,''), COALESCE(host(ip),''), COALESCE(speed_mbps,0)
		FROM interface WHERE device_id = $1 ORDER BY name`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IfaceDTO{}
	for rows.Next() {
		var i IfaceDTO
		if err := rows.Scan(&i.Name, &i.MAC, &i.IP, &i.SpeedMbps); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) deviceSoftware(ctx context.Context, id string) ([]SoftwareDTO, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT name, COALESCE(version,''), COALESCE(vendor,'')
		FROM software WHERE device_id = $1 ORDER BY lower(name)`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SoftwareDTO{}
	for rows.Next() {
		var sw SoftwareDTO
		if err := rows.Scan(&sw.Name, &sw.Version, &sw.Vendor); err != nil {
			return nil, err
		}
		out = append(out, sw)
	}
	return out, rows.Err()
}

func (s *Store) deviceUsers(ctx context.Context, id string) ([]UserDTO, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT username, COALESCE(full_name,''), last_logon, is_local
		FROM user_account WHERE device_id = $1 ORDER BY username`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserDTO{}
	for rows.Next() {
		var u UserDTO
		if err := rows.Scan(&u.Username, &u.FullName, &u.LastLogon, &u.IsLocal); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// SoftwareAgg is a rollup row: how many devices have a given software/version.
type SoftwareAgg struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	DeviceCount int    `json:"device_count"`
}

// SoftwareRollup aggregates installed software by name+version.
func (s *Store) SoftwareRollup(ctx context.Context, query string, limit int) ([]SoftwareAgg, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	var b argBuilder
	where := "1=1"
	if q := strings.TrimSpace(query); q != "" {
		where = "name ILIKE " + b.next("%"+q+"%")
	}
	lim := b.next(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT name, COALESCE(version,''), count(DISTINCT device_id) AS device_count
		FROM software WHERE `+where+`
		GROUP BY name, version
		ORDER BY device_count DESC, lower(name)
		LIMIT `+lim, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SoftwareAgg
	for rows.Next() {
		var a SoftwareAgg
		if err := rows.Scan(&a.Name, &a.Version, &a.DeviceCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ChangeRow is one entry in the change timeline.
type ChangeRow struct {
	DeviceID string    `json:"device_id"`
	Hostname string    `json:"hostname"`
	Field    string    `json:"field"`
	OldValue string    `json:"old_value"`
	NewValue string    `json:"new_value"`
	TS       time.Time `json:"ts"`
}

// ListChanges returns recent change events, optionally for one device.
func (s *Store) ListChanges(ctx context.Context, deviceID string, limit int) ([]ChangeRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var b argBuilder
	where := "1=1"
	if deviceID != "" {
		where = "c.device_id = " + b.next(deviceID)
	}
	lim := b.next(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT c.device_id, COALESCE(d.hostname, host(d.primary_ip), ''),
		       c.field, COALESCE(c.old_value,''), COALESCE(c.new_value,''), c.ts
		FROM change_event c
		JOIN device d ON d.id = c.device_id
		WHERE `+where+`
		ORDER BY c.ts DESC
		LIMIT `+lim, b.args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChangeRow
	for rows.Next() {
		var c ChangeRow
		if err := rows.Scan(&c.DeviceID, &c.Hostname, &c.Field, &c.OldValue, &c.NewValue, &c.TS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ScanRow summarizes a scan run.
type ScanRow struct {
	ID         string     `json:"id"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Status     string     `json:"status"`
	Discovered int        `json:"discovered"`
	HostsFound int        `json:"hosts_found"`
	Error      string     `json:"error"`
}

const scanRowCols = `id, started_at, finished_at, status, discovered, hosts_found, COALESCE(error,'')`

func scanScan(row interface{ Scan(...any) error }, r *ScanRow) error {
	return row.Scan(&r.ID, &r.StartedAt, &r.FinishedAt, &r.Status, &r.Discovered, &r.HostsFound, &r.Error)
}

// ListScans returns recent scan runs.
func (s *Store) ListScans(ctx context.Context, limit int) ([]ScanRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+scanRowCols+` FROM scan_run ORDER BY started_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScanRow
	for rows.Next() {
		var r ScanRow
		if err := scanScan(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetScanRun returns a single scan run, or (nil, nil) if not found.
func (s *Store) GetScanRun(ctx context.Context, id string) (*ScanRow, error) {
	var r ScanRow
	err := scanScan(s.pool.QueryRow(ctx, `SELECT `+scanRowCols+` FROM scan_run WHERE id = $1`, id), &r)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ScanEventRow is one per-host progress event.
type ScanEventRow struct {
	TS        time.Time `json:"ts"`
	Addr      string    `json:"addr"`
	Class     string    `json:"class"`
	Collector string    `json:"collector"`
	Status    string    `json:"status"`
	Changes   int       `json:"changes"`
	Error     string    `json:"error"`
}

// ListScanEvents returns a run's per-host events, newest first.
func (s *Store) ListScanEvents(ctx context.Context, runID string, limit int) ([]ScanEventRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ts, addr, COALESCE(device_class,''), COALESCE(collector,''), status, changes, COALESCE(error,'')
		FROM scan_event WHERE scan_run_id = $1 ORDER BY ts DESC LIMIT $2`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScanEventRow{}
	for rows.Next() {
		var e ScanEventRow
		if err := rows.Scan(&e.TS, &e.Addr, &e.Class, &e.Collector, &e.Status, &e.Changes, &e.Error); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Stats is the dashboard summary.
type Stats struct {
	TotalDevices  int            `json:"total_devices"`
	SeenLast24h   int            `json:"seen_last_24h"`
	ByType        map[string]int `json:"by_type"`
	LastScanAt    *time.Time     `json:"last_scan_at"`
	TotalSoftware int            `json:"total_software_titles"`
}

// Stats computes dashboard summary numbers.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	st.ByType = map[string]int{}

	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM device`).Scan(&st.TotalDevices); err != nil {
		return st, err
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM device WHERE last_seen > now() - interval '24 hours'`,
	).Scan(&st.SeenLast24h); err != nil {
		return st, err
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT count(DISTINCT name) FROM software`,
	).Scan(&st.TotalSoftware); err != nil {
		return st, err
	}

	rows, err := s.pool.Query(ctx, `SELECT device_type, count(*) FROM device GROUP BY device_type`)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var n int
		if err := rows.Scan(&t, &n); err != nil {
			return st, err
		}
		st.ByType[t] = n
	}
	if err := rows.Err(); err != nil {
		return st, err
	}

	var last *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(started_at) FROM scan_run`).Scan(&last); err != nil {
		return st, err
	}
	st.LastScanAt = last
	return st, nil
}
