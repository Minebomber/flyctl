package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountsFile_AddOrUpdateAccount(t *testing.T) {
	af := &AccountsFile{}

	// Add first account
	acc1 := Account{
		Email:       "user1@example.com",
		AccessToken: "token1",
		LastLogin:   time.Now(),
	}
	af.AddOrUpdateAccount(acc1)

	assert.Equal(t, 1, af.AccountCount())
	assert.Equal(t, "user1@example.com", af.Active)
	assert.True(t, af.HasAccounts())

	// Add second account
	acc2 := Account{
		Email:       "user2@example.com",
		AccessToken: "token2",
		LastLogin:   time.Now(),
	}
	af.AddOrUpdateAccount(acc2)

	assert.Equal(t, 2, af.AccountCount())
	assert.Equal(t, "user2@example.com", af.Active) // New account becomes active

	// Update first account
	acc1Updated := Account{
		Email:       "user1@example.com",
		AccessToken: "token1-updated",
		LastLogin:   time.Now(),
	}
	af.AddOrUpdateAccount(acc1Updated)

	assert.Equal(t, 2, af.AccountCount()) // Still 2 accounts
	assert.Equal(t, "user1@example.com", af.Active) // Updated account becomes active

	account, err := af.GetAccount("user1@example.com")
	require.NoError(t, err)
	assert.Equal(t, "token1-updated", account.AccessToken)
}

func TestAccountsFile_RemoveAccount(t *testing.T) {
	af := &AccountsFile{
		Active: "user1@example.com",
		Accounts: []Account{
			{Email: "user1@example.com", AccessToken: "token1"},
			{Email: "user2@example.com", AccessToken: "token2"},
		},
	}

	// Remove active account
	err := af.RemoveAccount("user1@example.com")
	require.NoError(t, err)

	assert.Equal(t, 1, af.AccountCount())
	assert.Equal(t, "user2@example.com", af.Active) // Auto-switched to remaining account

	// Remove non-existent account
	err = af.RemoveAccount("nonexistent@example.com")
	assert.ErrorIs(t, err, ErrAccountNotFound)

	// Remove last account
	err = af.RemoveAccount("user2@example.com")
	require.NoError(t, err)

	assert.Equal(t, 0, af.AccountCount())
	assert.Equal(t, "", af.Active)
	assert.False(t, af.HasAccounts())
}

func TestAccountsFile_SetActive(t *testing.T) {
	af := &AccountsFile{
		Active: "user1@example.com",
		Accounts: []Account{
			{Email: "user1@example.com", AccessToken: "token1"},
			{Email: "user2@example.com", AccessToken: "token2"},
		},
	}

	// Switch to existing account
	err := af.SetActive("user2@example.com")
	require.NoError(t, err)
	assert.Equal(t, "user2@example.com", af.Active)

	// Try to switch to non-existent account
	err = af.SetActive("nonexistent@example.com")
	assert.ErrorIs(t, err, ErrAccountNotFound)
}

func TestAccountsFile_GetActiveAccount(t *testing.T) {
	// Empty accounts
	af := &AccountsFile{}
	_, err := af.GetActiveAccount()
	assert.ErrorIs(t, err, ErrNoAccounts)

	// With accounts but no active set
	af = &AccountsFile{
		Accounts: []Account{
			{Email: "user1@example.com", AccessToken: "token1"},
		},
	}
	account, err := af.GetActiveAccount()
	require.NoError(t, err)
	assert.Equal(t, "user1@example.com", account.Email) // Returns first account

	// With active set
	af.Active = "user1@example.com"
	account, err = af.GetActiveAccount()
	require.NoError(t, err)
	assert.Equal(t, "user1@example.com", account.Email)
}

func TestLoadAndSaveAccounts(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create accounts
	af := &AccountsFile{
		Active: "user1@example.com",
		Accounts: []Account{
			{
				Email:       "user1@example.com",
				AccessToken: "token1",
				MetricsToken: "metrics1",
				LastLogin:   time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			},
			{
				Email:       "user2@example.com",
				AccessToken: "token2",
				LastLogin:   time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC),
			},
		},
	}

	// Save accounts
	err := SaveAccounts(tmpDir, af)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(filepath.Join(tmpDir, AccountsFileName))
	require.NoError(t, err)

	// Load accounts
	loaded, err := LoadAccounts(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, "user1@example.com", loaded.Active)
	assert.Equal(t, 2, loaded.AccountCount())

	acc1, err := loaded.GetAccount("user1@example.com")
	require.NoError(t, err)
	assert.Equal(t, "token1", acc1.AccessToken)
	assert.Equal(t, "metrics1", acc1.MetricsToken)

	acc2, err := loaded.GetAccount("user2@example.com")
	require.NoError(t, err)
	assert.Equal(t, "token2", acc2.AccessToken)
}

func TestLoadAccounts_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Load from non-existent directory
	af, err := LoadAccounts(tmpDir)
	require.NoError(t, err)

	assert.Equal(t, 0, af.AccountCount())
	assert.False(t, af.HasAccounts())
}
