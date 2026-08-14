package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"mikrotool/internal/store"
)

func TestMigrateLegacyDataAndInterruptedSession(t *testing.T) {
	config := t.TempDir()
	legacy := filepath.Join(config, "Wiretool")
	destination := filepath.Join(config, "Mikrotool")
	if err := os.MkdirAll(filepath.Join(legacy, "active"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "sites.csv"), []byte("ip_address,company_code,site_name\n192.0.2.1,ACME,Main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	recovery := store.Recovery{
		Host: "192.0.2.1", SSHPort: 22, Username: "admin",
		PeerName: "mikrotool-admin-010203040506", PublicKey: key.PublicKey().String(),
		TunnelName: "mt-010203040506", Interface: "utun9",
		ConfigPath: filepath.Join(legacy, "active", "mt-010203040506.conf"),
	}
	if err := os.WriteFile(recovery.ConfigPath, []byte("private config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.NewRecoveryStore(filepath.Join(legacy, "active-session.json")).Save(recovery); err != nil {
		t.Fatal(err)
	}
	if errs := migrateLegacyData(config, destination); len(errs) != 0 {
		t.Fatalf("migration errors: %v", errs)
	}
	if _, err := os.Stat(filepath.Join(destination, "sites.csv")); err != nil {
		t.Fatal(err)
	}
	migrated, err := store.NewRecoveryStore(filepath.Join(destination, "active-session.json")).Load()
	if err != nil || migrated == nil {
		t.Fatalf("load migrated recovery: value=%#v err=%v", migrated, err)
	}
	wantConfig := filepath.Join(destination, "active", "mt-010203040506.conf")
	if migrated.ConfigPath != wantConfig {
		t.Fatalf("unexpected migrated config path %q", migrated.ConfigPath)
	}
	if data, err := os.ReadFile(wantConfig); err != nil || string(data) != "private config" {
		t.Fatalf("migrated private config data=%q err=%v", data, err)
	}
}

func TestLegacyCopyRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	link := filepath.Join(directory, "link")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := copyLegacyRegularFile(link, filepath.Join(directory, "copy"), 1<<10); err == nil {
		t.Fatal("legacy migration accepted a symlink")
	}
}
