package config

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/superfly/flyctl/internal/filemu"
)

// AccountsFileName is the name of the accounts file.
const AccountsFileName = "accounts.yml"

// Account represents a single authorized Fly.io account.
type Account struct {
	Email        string    `yaml:"email"`
	AccessToken  string    `yaml:"access_token"`
	MetricsToken string    `yaml:"metrics_token,omitempty"`
	LastLogin    time.Time `yaml:"last_login,omitempty"`
}

// AccountsFile represents the multi-account storage file.
type AccountsFile struct {
	Active   string    `yaml:"active"`
	Accounts []Account `yaml:"accounts"`
}

// ErrNoAccounts is returned when no accounts are configured.
var ErrNoAccounts = errors.New("no accounts configured")

// ErrAccountNotFound is returned when the specified account doesn't exist.
var ErrAccountNotFound = errors.New("account not found")

// AccountsFilePath returns the path to the accounts file in the given config directory.
func AccountsFilePath(configDir string) string {
	return filepath.Join(configDir, AccountsFileName)
}

// LoadAccounts loads the accounts file from the given config directory.
// Returns an empty AccountsFile if the file doesn't exist.
func LoadAccounts(configDir string) (*AccountsFile, error) {
	path := AccountsFilePath(configDir)

	var af AccountsFile
	if err := unmarshalAccounts(path, &af); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &AccountsFile{}, nil
		}
		return nil, err
	}

	return &af, nil
}

// SaveAccounts saves the accounts file to the given config directory.
func SaveAccounts(configDir string, af *AccountsFile) error {
	path := AccountsFilePath(configDir)
	return marshalAccounts(path, af)
}

// GetActiveAccount returns the currently active account.
// Returns ErrNoAccounts if no accounts are configured.
func (af *AccountsFile) GetActiveAccount() (*Account, error) {
	if len(af.Accounts) == 0 {
		return nil, ErrNoAccounts
	}

	// If no active account is set, return the first one
	if af.Active == "" && len(af.Accounts) > 0 {
		return &af.Accounts[0], nil
	}

	for i := range af.Accounts {
		if af.Accounts[i].Email == af.Active {
			return &af.Accounts[i], nil
		}
	}

	// If active account not found but we have accounts, return first one
	if len(af.Accounts) > 0 {
		return &af.Accounts[0], nil
	}

	return nil, ErrNoAccounts
}

// GetAccount returns the account with the given email.
func (af *AccountsFile) GetAccount(email string) (*Account, error) {
	for i := range af.Accounts {
		if af.Accounts[i].Email == email {
			return &af.Accounts[i], nil
		}
	}
	return nil, ErrAccountNotFound
}

// AddOrUpdateAccount adds a new account or updates an existing one.
// If the account already exists (by email), it will be updated.
// The new/updated account becomes the active account.
func (af *AccountsFile) AddOrUpdateAccount(account Account) {
	for i := range af.Accounts {
		if af.Accounts[i].Email == account.Email {
			af.Accounts[i] = account
			af.Active = account.Email
			return
		}
	}

	// Account doesn't exist, add it
	af.Accounts = append(af.Accounts, account)
	af.Active = account.Email
}

// RemoveAccount removes the account with the given email.
// If the removed account was active, switches to the first available account.
// Returns ErrAccountNotFound if the account doesn't exist.
func (af *AccountsFile) RemoveAccount(email string) error {
	idx := -1
	for i := range af.Accounts {
		if af.Accounts[i].Email == email {
			idx = i
			break
		}
	}

	if idx == -1 {
		return ErrAccountNotFound
	}

	// Remove the account
	af.Accounts = append(af.Accounts[:idx], af.Accounts[idx+1:]...)

	// If removed account was active, switch to first available or clear
	if af.Active == email {
		if len(af.Accounts) > 0 {
			af.Active = af.Accounts[0].Email
		} else {
			af.Active = ""
		}
	}

	return nil
}

// SetActive sets the active account to the one with the given email.
// Returns ErrAccountNotFound if the account doesn't exist.
func (af *AccountsFile) SetActive(email string) error {
	for _, acc := range af.Accounts {
		if acc.Email == email {
			af.Active = email
			return nil
		}
	}
	return ErrAccountNotFound
}

// HasAccounts returns true if there are any accounts configured.
func (af *AccountsFile) HasAccounts() bool {
	return len(af.Accounts) > 0
}

// AccountCount returns the number of configured accounts.
func (af *AccountsFile) AccountCount() int {
	return len(af.Accounts)
}

func accountsLockPath(configDir string) string {
	return filepath.Join(configDir, "flyctl.accounts.lock")
}

func unmarshalAccounts(path string, v interface{}) (err error) {
	configDir := filepath.Dir(path)
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.RLock(context.Background(), accountsLockPath(configDir)); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	var f *os.File
	if f, err = os.Open(path); err != nil {
		return
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()

	err = yaml.NewDecoder(f).Decode(v)

	return
}

func marshalAccounts(path string, v interface{}) (err error) {
	configDir := filepath.Dir(path)
	var unlock filemu.UnlockFunc
	if unlock, err = filemu.Lock(context.Background(), accountsLockPath(configDir)); err != nil {
		return
	}
	defer func() {
		if e := unlock(); err == nil {
			err = e
		}
	}()

	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}

	err = os.WriteFile(path, data, 0o600)
	return
}

// SyncActiveAccountToConfig syncs the active account's token to the main config file.
// This provides backward compatibility with code that reads from config.yml directly.
func SyncActiveAccountToConfig(configDir string) error {
	af, err := LoadAccounts(configDir)
	if err != nil {
		return err
	}

	if !af.HasAccounts() {
		return nil
	}

	account, err := af.GetActiveAccount()
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, FileName)

	// Set the access token
	if err := SetAccessToken(configPath, account.AccessToken); err != nil {
		return err
	}

	// Set the metrics token if available
	if account.MetricsToken != "" {
		if err := SetMetricsToken(configPath, account.MetricsToken); err != nil {
			return err
		}
	}

	// Set the last login timestamp
	if !account.LastLogin.IsZero() {
		if err := SetLastLogin(configPath, account.LastLogin); err != nil {
			return err
		}
	}

	return nil
}
