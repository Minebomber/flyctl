package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newSwitch() *cobra.Command {
	const (
		long = `Switch the active Fly.io account.
If no email is provided, displays an interactive prompt to select an account.
`
		short = "Switch to a different account"
	)

	cmd := command.New("switch [email]", short, long, runSwitch)
	cmd.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runSwitch(ctx context.Context) error {
	configDir := state.ConfigDirectory(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	af, err := config.LoadAccounts(configDir)
	if err != nil {
		return fmt.Errorf("failed to load accounts: %w", err)
	}

	if !af.HasAccounts() {
		return errors.New("no accounts configured. Use 'fly auth login' to add an account")
	}

	if af.AccountCount() == 1 {
		fmt.Fprintf(io.Out, "Only one account configured: %s\n", colorize.Bold(af.Accounts[0].Email))
		return nil
	}

	// Get the target email from args or prompt
	var targetEmail string

	if arg := flag.FirstArg(ctx); arg != "" {
		targetEmail = arg
	} else {
		// Interactive selection
		options := make([]string, 0, len(af.Accounts))
		for _, acc := range af.Accounts {
			label := acc.Email
			if acc.Email == af.Active {
				label = acc.Email + " (current)"
			}
			options = append(options, label)
		}

		selectedIdx := 0
		if err := prompt.Select(ctx, &selectedIdx, "Select account:", "", options...); err != nil {
			if prompt.IsNonInteractive(err) {
				return errors.New("email argument required when not running interactively")
			}
			return err
		}

		targetEmail = af.Accounts[selectedIdx].Email
	}

	// Check if already on this account
	if targetEmail == af.Active {
		fmt.Fprintf(io.Out, "Already using account: %s\n", colorize.Bold(targetEmail))
		return nil
	}

	// Switch to the new account
	if err := af.SetActive(targetEmail); err != nil {
		if errors.Is(err, config.ErrAccountNotFound) {
			return fmt.Errorf("account '%s' not found. Use 'fly auth accounts' to list available accounts", targetEmail)
		}
		return err
	}

	// Save the accounts file
	if err := config.SaveAccounts(configDir, af); err != nil {
		return fmt.Errorf("failed to save accounts: %w", err)
	}

	// Sync the new active account to config.yml
	if err := config.SyncActiveAccountToConfig(configDir); err != nil {
		return fmt.Errorf("failed to sync account config: %w", err)
	}

	// Kill the agent so it picks up the new credentials
	if ac, err := agent.DefaultClient(ctx); err == nil {
		_ = ac.Kill(ctx)
	}

	fmt.Fprintf(io.Out, "Switched to account: %s\n", colorize.Green(targetEmail))

	return nil
}
