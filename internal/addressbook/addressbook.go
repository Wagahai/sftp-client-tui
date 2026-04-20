package addressbook

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/argon2"

	"github.com/aaronkiula/sftp-tui/internal/config"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
	nonceLen     = 12
)

type Entry struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Host          string `json:"host"`
	Port          string `json:"port"`
	User          string `json:"user"`
	AuthType      string `json:"auth_type"` // "password" | "key"
	Password      string `json:"password"`
	KeyPath       string `json:"key_path"`
	KeyPassphrase string `json:"key_passphrase"`
}

type wireBook struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

type Book struct {
	entries  []Entry
	password []byte // zeroed on Zero(); used to re-derive key on each save
	path     string
}

var ErrWrongPassword = errors.New("wrong master password")

func Open(masterPassword []byte) (*Book, error) {
	path, err := config.AddressBookPath()
	if err != nil {
		return nil, err
	}

	pw := make([]byte, len(masterPassword))
	copy(pw, masterPassword)

	b := &Book{path: path, password: pw}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// New address book — save empty book to create the file.
		if err := b.save(); err != nil {
			return nil, err
		}
		return b, nil
	}
	if err != nil {
		return nil, err
	}

	if len(data) < saltLen+nonceLen+1 {
		return nil, fmt.Errorf("addressbook file corrupt")
	}

	entries, err := decryptBook(masterPassword, data)
	if err != nil {
		return nil, ErrWrongPassword
	}
	b.entries = entries
	return b, nil
}

func (b *Book) List() []Entry {
	out := make([]Entry, len(b.entries))
	copy(out, b.entries)
	return out
}

func (b *Book) Add(e Entry) error {
	if e.ID == "" {
		e.ID = randomID()
	}
	if e.Port == "" {
		e.Port = "22"
	}
	b.entries = append(b.entries, e)
	return b.save()
}

func (b *Book) Update(e Entry) error {
	for i, existing := range b.entries {
		if existing.ID == e.ID {
			b.entries[i] = e
			return b.save()
		}
	}
	return fmt.Errorf("entry %s not found", e.ID)
}

func (b *Book) Delete(id string) error {
	for i, e := range b.entries {
		if e.ID == id {
			b.entries = append(b.entries[:i], b.entries[i+1:]...)
			return b.save()
		}
	}
	return fmt.Errorf("entry %s not found", id)
}

// Zero wipes the master password and blanks sensitive fields in all entries.
// Call on app exit or address book lock. Go strings are immutable so we cannot
// zero their backing memory, but we drop all references and nil the slice so
// the GC can reclaim them.
func (b *Book) Zero() {
	for i := range b.password {
		b.password[i] = 0
	}
	for i := range b.entries {
		b.entries[i].Password = ""
		b.entries[i].KeyPassphrase = ""
	}
	b.entries = nil
}

func (b *Book) save() error {
	wk := wireBook{Version: 1, Entries: b.entries}
	plaintext, err := json.Marshal(wk)
	if err != nil {
		return err
	}

	salt := make([]byte, saltLen)
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	key := deriveKey(b.password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	out := make([]byte, 0, saltLen+nonceLen+len(ciphertext))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, ciphertext...)

	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}

func decryptBook(password, data []byte) ([]Entry, error) {
	salt := data[:saltLen]
	nonce := data[saltLen : saltLen+nonceLen]
	ciphertext := data[saltLen+nonceLen:]

	key := deriveKey(password, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrWrongPassword
	}

	var wk wireBook
	if err := json.Unmarshal(plaintext, &wk); err != nil {
		return nil, err
	}
	return wk.Entries, nil
}

func deriveKey(password, salt []byte) []byte {
	return argon2.IDKey(password, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

func randomID() string {
	b := make([]byte, 8)
	io.ReadFull(rand.Reader, b) //nolint:errcheck
	return fmt.Sprintf("%x", b)
}
