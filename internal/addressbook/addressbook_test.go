package addressbook

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempPath(t *testing.T, fn func(path string)) {
	t.Helper()
	dir := t.TempDir()
	// Patch the config path by directly setting the path on Book structs.
	_ = dir
	fn(filepath.Join(dir, "addressbook.enc"))
}

// openWithPath bypasses config.AddressBookPath for testing.
func openWithPath(password []byte, path string) (*Book, error) {
	pw := make([]byte, len(password))
	copy(pw, password)

	b := &Book{path: path, password: pw}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := b.save(); err != nil {
			return nil, err
		}
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) < saltLen+nonceLen+1 {
		return nil, ErrWrongPassword
	}
	entries, err := decryptBook(password, data)
	if err != nil {
		return nil, ErrWrongPassword
	}
	b.entries = entries
	return b, nil
}

func TestNewBook(t *testing.T) {
	withTempPath(t, func(path string) {
		b, err := openWithPath([]byte("secret"), path)
		if err != nil {
			t.Fatalf("unexpected error opening new book: %v", err)
		}
		if len(b.List()) != 0 {
			t.Error("expected empty book")
		}
	})
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	withTempPath(t, func(path string) {
		b, _ := openWithPath([]byte("mysecret"), path)

		entry := Entry{
			Label:    "My Server",
			Host:     "example.com",
			Port:     "22",
			User:     "admin",
			AuthType: "password",
			Password: "hunter2",
		}
		if err := b.Add(entry); err != nil {
			t.Fatalf("Add: %v", err)
		}

		// Reopen with correct password.
		b2, err := openWithPath([]byte("mysecret"), path)
		if err != nil {
			t.Fatalf("reopen: %v", err)
		}
		entries := b2.List()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
		if entries[0].Host != "example.com" {
			t.Errorf("expected host example.com, got %s", entries[0].Host)
		}
		if entries[0].Password != "hunter2" {
			t.Errorf("password not persisted correctly")
		}
	})
}

func TestWrongPassword(t *testing.T) {
	withTempPath(t, func(path string) {
		b, _ := openWithPath([]byte("correct"), path)
		b.Add(Entry{Host: "h", User: "u", AuthType: "password"}) //nolint:errcheck

		_, err := openWithPath([]byte("wrong"), path)
		if err != ErrWrongPassword {
			t.Errorf("expected ErrWrongPassword, got %v", err)
		}
	})
}

func TestAddUpdateDelete(t *testing.T) {
	withTempPath(t, func(path string) {
		b, _ := openWithPath([]byte("pw"), path)

		e := Entry{Host: "host1", User: "user1", AuthType: "password", Port: "22"}
		b.Add(e) //nolint:errcheck

		entries := b.List()
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry")
		}
		id := entries[0].ID

		// Update.
		entries[0].Host = "host2"
		if err := b.Update(entries[0]); err != nil {
			t.Fatalf("Update: %v", err)
		}
		if b.List()[0].Host != "host2" {
			t.Error("update did not persist")
		}

		// Delete.
		if err := b.Delete(id); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if len(b.List()) != 0 {
			t.Error("expected empty book after delete")
		}
	})
}

func TestEmptyPassword(t *testing.T) {
	withTempPath(t, func(path string) {
		b, err := openWithPath([]byte(""), path)
		if err != nil {
			t.Fatalf("empty password should work: %v", err)
		}
		b.Add(Entry{Host: "h", User: "u", AuthType: "key"}) //nolint:errcheck

		b2, err := openWithPath([]byte(""), path)
		if err != nil {
			t.Fatalf("reopen with empty password: %v", err)
		}
		if len(b2.List()) != 1 {
			t.Error("expected 1 entry")
		}
	})
}

func TestKeyAuth(t *testing.T) {
	withTempPath(t, func(path string) {
		b, _ := openWithPath([]byte("pw"), path)
		e := Entry{
			Label:         "Key Server",
			Host:          "keyserver.example.com",
			Port:          "2222",
			User:          "deploy",
			AuthType:      "key",
			KeyPath:       "/home/user/.ssh/id_ed25519",
			KeyPassphrase: "keypass",
		}
		b.Add(e) //nolint:errcheck

		b2, _ := openWithPath([]byte("pw"), path)
		got := b2.List()
		if len(got) != 1 {
			t.Fatal("expected 1 entry")
		}
		if got[0].AuthType != "key" {
			t.Error("auth type not persisted")
		}
		if got[0].KeyPath != "/home/user/.ssh/id_ed25519" {
			t.Error("key path not persisted")
		}
	})
}
