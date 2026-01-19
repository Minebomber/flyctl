package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newAccounts() *cobra.Command {
	const (
		long = `List all authenticated Fly.io accounts.
The currently active account is marked with an asterisk (*).
Use 'fly auth switch' to change the active account.
`
		short = "List all authenticated accounts"
	)

	cmd := command.New("accounts", short, long, runAccounts)

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runAccounts(ctx context.Context) error {
	configDir := state.ConfigDirectory(ctx)
	io := iostreams.FromContext(ctx)
	cfg := config.FromContext(ctx)

	af, err := config.LoadAccounts(configDir)
	if err != nil {
		return fmt.Errorf("failed to load accounts: %w", err)
	}

	if !af.HasAccounts() {
		fmt.Fprintln(io.Out, "No accounts configured. Use 'fly auth login' to add an account.")
		return nil
	}

	if cfg.JSONOutput {
		type jsonAccount struct {
			Email    string `json:"email"`
			Active   bool   `json:"active"`
			LastLogin string `json:"last_login,omitempty"`
		}

		accounts := make([]jsonAccount, 0, len(af.Accounts))
		for _, acc := range af.Accounts {
			lastLogin := ""
			if !acc.LastLogin.IsZero() {
				lastLogin = acc.LastLogin.Format("2006-01-02 15:04:05")
			}
			accounts = append(accounts, jsonAccount{
				Email:    acc.Email,
				Active:   acc.Email == af.Active,
				LastLogin: lastLogin,
			})
		}

		return render.JSON(io.Out, accounts)
	}

	colorize := io.ColorScheme()

	fmt.Fprintln(io.Out, "Authenticated accounts:")
	fmt.Fprintln(io.Out)

	for _, acc := range af.Accounts {
		marker := "  "
		email := acc.Email
		if acc.Email == af.Active {
			marker = colorize.Green("* ")
			email = colorize.Bold(acc.Email)
		}

		lastLogin := ""
		if !acc.LastLogin.IsZero() {
			lastLogin = fmt.Sprintf(" (last login: %s)", acc.LastLogin.Format("2006-01-02"))
		}

		fmt.Fprintf(io.Out, "%s%s%s\n", marker, email, lastLogin)
	}

	fmt.Fprintln(io.Out)
	fmt.Fprintf(io.Out, "Active account: %s\n", colorize.Bold(af.Active))

	return nil
}
