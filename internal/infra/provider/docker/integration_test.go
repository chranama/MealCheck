package docker

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/chranama/MealCheck/internal/infra/provider"
	"github.com/moby/moby/client"
)

// Explicit opt-in: uses only unique test names and removes its empty resources.
func TestEngineOwnershipAndVolumeIncarnation(t *testing.T) {
	endpoint := os.Getenv("MEALCHECK_CONTROLLER_TEST_ENGINE")
	if endpoint == "" {
		t.Skip("dedicated local engine endpoint required")
	}
	d, e := New(endpoint, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	r := provider.Resource{Kind: "network", Role: "network", Name: "mc-test-collision-" + suffix, InstallationID: "test-owner", DeploymentID: "test", Fingerprint: "test"}
	if _, e = d.cli.NetworkCreate(ctx, r.Name, client.NetworkCreateOptions{Driver: "bridge"}); e != nil {
		t.Fatal(e)
	}
	defer d.cli.NetworkRemove(context.Background(), r.Name, client.NetworkRemoveOptions{})
	if e = d.Ensure(ctx, r); e == nil {
		t.Fatal("unowned network adopted")
	}
	if e = d.Remove(ctx, r); e == nil {
		t.Fatal("unowned network removed")
	}
	r.Kind = "volume"
	r.Role = "database-volume"
	r.Name = "mc-test-volume-" + suffix
	if e = d.Ensure(ctx, r); e != nil {
		t.Fatal(e)
	}
	defer d.cli.VolumeRemove(context.Background(), r.Name, client.VolumeRemoveOptions{})
	first, e := d.inspect(ctx, r)
	if e != nil || first.ID == "" {
		t.Fatalf("first %v %v", first, e)
	}
	if e = d.Ensure(ctx, r); e != nil {
		t.Fatal(e)
	}
	same, e := d.inspect(ctx, r)
	if e != nil || same.ID != first.ID {
		t.Fatal("idempotent ensure changed identity")
	}
	if _, e = d.cli.VolumeRemove(ctx, r.Name, client.VolumeRemoveOptions{}); e != nil {
		t.Fatal(e)
	}
	if e = d.Ensure(ctx, r); e != nil {
		t.Fatal(e)
	}
	next, e := d.inspect(ctx, r)
	if e != nil || next.ID == first.ID {
		t.Fatal("recreated volume reused incarnation")
	}
	if e = d.Remove(ctx, r); e == nil {
		t.Fatal("provider purged retained volume")
	}
}
