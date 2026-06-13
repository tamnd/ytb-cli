package cli

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newCommentsCmd(app *App) *cobra.Command {
	var (
		replies bool
		all     bool
		sortBy  string
	)
	cmd := &cobra.Command{
		Use:   "comments <video-id|url>",
		Short: "Comments and replies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			store, err := app.Store()
			if err != nil {
				return err
			}
			opt := youtube.CommentOptions{
				Max:      app.Limit,
				MaxPages: app.MaxPages,
				Replies:  replies,
				Sort:     sortBy,
			}
			if all {
				opt.Max = 0
				opt.MaxPages = 0
			}
			var n int
			err = app.Client.StreamComments(ctx, args[0], opt, func(c youtube.Comment) error {
				n++
				if store != nil {
					_ = store.UpsertComment(c)
				}
				return app.Out.Emit(commentRow(c))
			})
			if err != nil && err != youtube.ErrStop {
				if errors.Is(err, youtube.ErrCommentsRestricted) {
					return partialErr("comments are hidden by Restricted Mode; YouTube applies this to some server and datacenter requests")
				}
				return err
			}
			if n == 0 {
				return noResults("no comments (or comments are disabled)")
			}
			return app.Out.Flush()
		},
	}
	f := cmd.Flags()
	f.BoolVar(&replies, "replies", false, "also fetch replies (parent_id set)")
	f.BoolVar(&all, "all", false, "remove the default cap")
	f.StringVar(&sortBy, "sort", "", "top|new")
	return cmd
}
