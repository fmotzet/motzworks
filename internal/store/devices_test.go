package store

import (
	"context"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stock3/motzworks/internal/model"
)

// testStore creates a clean motzworks_test database and migrates it. It skips
// the test if PostgreSQL is not reachable.
func testStore(t *testing.T) *Store {
	t.Helper()
	base := os.Getenv("MOTZWORKS_TEST_DSN")
	if base == "" {
		base = "host=localhost port=5432 user=motzworks password=motzworks dbname=motzworks sslmode=disable"
	}
	ctx := context.Background()

	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS motzworks_test WITH (FORCE)")
	if _, err := admin.Exec(ctx, "CREATE DATABASE motzworks_test"); err != nil {
		admin.Close()
		t.Fatalf("create test db: %v", err)
	}
	admin.Close()

	st, err := Open(ctx, replaceDBName(base, "motzworks_test"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func replaceDBName(dsn, name string) string {
	parts := strings.Fields(dsn)
	for i, p := range parts {
		if strings.HasPrefix(p, "dbname=") {
			parts[i] = "dbname=" + name
		}
	}
	return strings.Join(parts, " ")
}

func (s *Store) deviceCount(ctx context.Context, t *testing.T) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM device").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestUpsertInsertAndDedupBySerial(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	dev := model.Device{
		Type:      model.TypeLinux,
		Hostname:  "web01",
		PrimaryIP: netip.MustParseAddr("10.0.0.10"),
		Serial:    "SN-ABC-123",
		Source:    "ssh",
		Interfaces: []model.Interface{
			{Name: "eth0", MAC: "aa:bb:cc:dd:ee:01", IP: netip.MustParseAddr("10.0.0.10")},
		},
		OS:       &model.OSInfo{Family: "linux", Name: "Ubuntu", Version: "22.04"},
		Software: []model.Software{{Name: "nginx", Version: "1.24.0"}},
	}

	id1, changes, err := st.UpsertDevice(ctx, dev, "")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("new device should have no change events, got %v", changes)
	}

	// Re-scan: same serial, new hostname + OS upgrade. Must dedup to same id
	// and emit change events.
	dev.Hostname = "web01-renamed"
	dev.OS.Version = "24.04"
	id2, changes, err := st.UpsertDevice(ctx, dev, "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("dedup by serial failed: %s != %s", id1, id2)
	}
	if n := st.deviceCount(ctx, t); n != 1 {
		t.Fatalf("expected 1 device, got %d", n)
	}

	got := map[string]string{}
	for _, c := range changes {
		got[c.Field] = c.New
	}
	if got["hostname"] != "web01-renamed" {
		t.Errorf("expected hostname change, got %v", changes)
	}
	if got["os_version"] != "24.04" {
		t.Errorf("expected os_version change, got %v", changes)
	}
}

func TestDedupByMAC(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	dev := model.Device{
		Type:     model.TypeLinux,
		Hostname: "host-a",
		Interfaces: []model.Interface{
			{Name: "eth0", MAC: "11:22:33:44:55:66"},
		},
	}
	id1, _, err := st.UpsertDevice(ctx, dev, "")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Same MAC, different hostname, no serial → must match by MAC.
	dev.Hostname = "host-a-dhcp-changed"
	id2, _, err := st.UpsertDevice(ctx, dev, "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("dedup by MAC failed: %s != %s", id1, id2)
	}
	if n := st.deviceCount(ctx, t); n != 1 {
		t.Fatalf("expected 1 device, got %d", n)
	}
}

func TestCreateRelationship(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	hv, _, err := st.UpsertDevice(ctx, model.Device{Type: model.TypeHypervisor, Hostname: "pve1", Serial: "PVE-1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	vm, _, err := st.UpsertDevice(ctx, model.Device{Type: model.TypeVM, Hostname: "web01", Serial: "VM-1"}, "")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.CreateRelationship(ctx, hv, vm, "hosts-vm"); err != nil {
		t.Fatalf("create relationship: %v", err)
	}
	// Idempotent (ON CONFLICT DO NOTHING).
	if err := st.CreateRelationship(ctx, hv, vm, "hosts-vm"); err != nil {
		t.Fatalf("duplicate relationship: %v", err)
	}

	var n int
	if err := st.pool.QueryRow(ctx,
		"SELECT count(*) FROM relationship WHERE parent_id=$1 AND child_id=$2 AND kind='hosts-vm'", hv, vm,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 relationship row, got %d", n)
	}

	// Self-relationship and empty ids are no-ops, not errors.
	if err := st.CreateRelationship(ctx, hv, hv, "x"); err != nil {
		t.Errorf("self relationship should be a no-op: %v", err)
	}
}

func TestChildrenReplaced(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	dev := model.Device{
		Serial:   "SN-SW-1",
		Software: []model.Software{{Name: "pkg-a"}, {Name: "pkg-b"}, {Name: "pkg-c"}},
	}
	id, _, err := st.UpsertDevice(ctx, dev, "")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	dev.Software = []model.Software{{Name: "pkg-a"}}
	if _, _, err := st.UpsertDevice(ctx, dev, ""); err != nil {
		t.Fatalf("update: %v", err)
	}

	var n int
	if err := st.pool.QueryRow(ctx, "SELECT count(*) FROM software WHERE device_id = $1", id).Scan(&n); err != nil {
		t.Fatalf("count software: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected software replaced to 1 row, got %d", n)
	}
}
