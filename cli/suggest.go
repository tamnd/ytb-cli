package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func newSuggestCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suggest <query>",
		Short: "Search autocomplete suggestions",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			suggestions, err := app.Client.Suggest(ctx, strings.Join(args, " "))
			if err != nil {
				return err
			}
			if len(suggestions) == 0 {
				return noResults("no suggestions")
			}
			for _, s := range suggestions {
				if err := app.Out.Emit(suggestRow(s)); err != nil {
					return err
				}
			}
			return app.Out.Flush()
		},
	}
	return cmd
}
