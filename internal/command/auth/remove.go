package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newRemove() *cobra.Command {
	const (
		long = `Remove an authenticated account from the local configuration.
This will revoke the token and remove the account from the accounts list.
If the removed account was the active account, another account will be activated automatically.
`
		short = "Remove an authenticated account"
	)

	cmd := command.New("remove [email]", short, long, runRemove)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.Bool{
			Name:        "yes",
			Shorthand:   "y",
			Description: "Skip confirmation prompt",
		},
	)

	return cmd
}

func runRemove(ctx context.Context) error {
	configDir := state.ConfigDirectory(ctx)
	io := iostreams.FromContext(ctx)
	log := logger.FromContext(ctx)
	colorize := io.ColorScheme()

	af, err := config.LoadAccounts(configDir)
	if err != nil {
		return fmt.Errorf("failed to load accounts: %w", err)
	}

	if !af.HasAccounts() {
		return errors.New("no accounts configured")
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
		if err := prompt.Select(ctx, &selectedIdx, "Select account to remove:", "", options...); err != nil {
			if prompt.IsNonInteractive(err) {
				return errors.New("email argument required when not running interactively")
			}
			return err
		}

		targetEmail = af.Accounts[selectedIdx].Email
	}

	// Get the account to remove
	account, err := af.GetAccount(targetEmail)
	if err != nil {
		if errors.Is(err, config.ErrAccountNotFound) {
			return fmt.Errorf("account '%s' not found. Use 'fly auth accounts' to list available accounts", targetEmail)
		}
		return err
	}

	// Confirmation prompt
	if !flag.GetBool(ctx, "yes") {
		msg := fmt.Sprintf("Remove account '%s'?", targetEmail)
		if targetEmail == af.Active && af.AccountCount() > 1 {
			msg = fmt.Sprintf("Remove account '%s'? Another account will become active.", targetEmail)
		} else if af.AccountCount() == 1 {
			msg = fmt.Sprintf("Remove account '%s'? This is your only account and you will be logged out.", targetEmail)
		}

		confirmed, err := prompt.Confirm(ctx, msg)
		if err != nil {
			if prompt.IsNonInteractive(err) {
				return errors.New("use --yes to skip confirmation prompt in non-interactive mode")
			}
			return err
		}

		if !confirmed {
			fmt.Fprintln(io.Out, "Cancelled")
			return nil
		}
	}

	wasActive := targetEmail == af.Active

	// Try to revoke the token on the server
	if account.AccessToken != "" {
		client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
			AccessToken: account.AccessToken,
		})

		if client.Authenticated() {
			resp, err := gql.LogOut(ctx, client.GenqClient())
			if err != nil || !resp.LogOut.Ok {
				log.Warnf("Unable to revoke token for %s", targetEmail)
			}
			if err != nil {
				log.Debug(err.Error())
			}
		}
	}

	// Remove the account
	if err := af.RemoveAccount(targetEmail); err != nil {
		return err
	}

	// Save the accounts file
	if err := config.SaveAccounts(configDir, af); err != nil {
		return fmt.Errorf("failed to save accounts: %w", err)
	}

	// If the removed account was active, sync the new active account or clear config
	if wasActive {
		if af.HasAccounts() {
			if err := config.SyncActiveAccountToConfig(configDir); err != nil {
				return fmt.Errorf("failed to sync account config: %w", err)
			}

			// Kill the agent so it picks up the new credentials
			if ac, err := agent.DefaultClient(ctx); err == nil {
				_ = ac.Kill(ctx)
			}

			fmt.Fprintf(io.Out, "Removed account: %s\n", colorize.Yellow(targetEmail))
			fmt.Fprintf(io.Out, "Switched to account: %s\n", colorize.Green(af.Active))
		} else {
			// No accounts left, clear config
			configPath := state.ConfigFile(ctx)
			if err := config.Clear(configPath); err != nil {
				return fmt.Errorf("failed to clear config: %w", err)
			}

			if ac, err := agent.DefaultClient(ctx); err == nil {
				_ = ac.Kill(ctx)
			}

			fmt.Fprintf(io.Out, "Removed account: %s\n", colorize.Yellow(targetEmail))
			fmt.Fprintln(io.Out, "No accounts remaining. Use 'fly auth login' to authenticate.")
		}
	} else {
		fmt.Fprintf(io.Out, "Removed account: %s\n", colorize.Yellow(targetEmail))
	}

	return nil
}
