package auth

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func newLogout() *cobra.Command {
	const (
		long = `Log the currently logged-in user out of the Fly platform.
To continue interacting with Fly, the user will need to log in again.
`
		short = "Logs out the currently logged in user"
	)

	return command.New("logout", short, long, runLogout)
}

func runLogout(ctx context.Context) (err error) {
	log := logger.FromContext(ctx)
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	configDir := state.ConfigDirectory(ctx)

	// Try to revoke the token on the server
	if c := flyutil.ClientFromContext(ctx); c.Authenticated() {
		resp, err := gql.LogOut(ctx, c.GenqClient())
		if err != nil || !resp.LogOut.Ok {
			log.Warnf("Unable to revoke token")
		}
		if err != nil {
			log.Debug(err.Error())
		}
	}

	// Kill the agent
	var ac *agent.Client
	if ac, err = agent.DefaultClient(ctx); err == nil {
		if err = ac.Kill(ctx); err != nil {
			err = fmt.Errorf("failed stopping agent: %w", err)
			return
		}
	}

	// Handle multi-account: remove current account and switch to another if available
	af, loadErr := config.LoadAccounts(configDir)
	if loadErr == nil && af.HasAccounts() {
		currentEmail := af.Active

		// Remove the current account
		if removeErr := af.RemoveAccount(currentEmail); removeErr == nil {
			// Save the accounts file
			if saveErr := config.SaveAccounts(configDir, af); saveErr != nil {
				log.Warnf("Failed to save accounts file: %v", saveErr)
			}

			if af.HasAccounts() {
				// Sync the new active account to config.yml
				if syncErr := config.SyncActiveAccountToConfig(configDir); syncErr != nil {
					log.Warnf("Failed to sync account config: %v", syncErr)
				}

				fmt.Fprintf(io.Out, "Logged out of %s\n", colorize.Yellow(currentEmail))
				fmt.Fprintf(io.Out, "Switched to account: %s\n", colorize.Green(af.Active))

				warnEnvVars(io.ErrOut)
				return nil
			}
		}
	}

	// No other accounts or couldn't load accounts file - clear everything
	path := state.ConfigFile(ctx)
	if err = config.Clear(path); err != nil {
		err = fmt.Errorf("failed clearing config file at %s: %w\n", path, err)
		return
	}

	fmt.Fprintln(io.Out, "Logged out successfully.")
	warnEnvVars(io.ErrOut)

	return
}

func warnEnvVars(out io.Writer) {
	single := func(key string) {
		fmt.Fprintf(out,
			"$%s is set in your environment; don't forget to remove it.\n", key)
	}

	keyExists := env.IsSet(config.APITokenEnvKey)
	tokenExists := env.IsSet(config.AccessTokenEnvKey)

	switch {
	case keyExists && tokenExists:
		const msg = "$%s & $%s are set in your environment; don't forget to remove them.\n"
		fmt.Fprintf(out, msg, config.APITokenEnvKey, config.AccessTokenEnvKey)
	case keyExists:
		single(config.APITokenEnvKey)
	case tokenExists:
		single(config.AccessTokenEnvKey)
	}
}
