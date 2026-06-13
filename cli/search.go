package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/tamnd/ytb-cli/youtube"
)

func newSearchCmd(app *App) *cobra.Command {
	var (
		typ        string
		sortBy     string
		duration   string
		uploadDate string
		hd         bool
		cc         bool
		creative   bool
		live       bool
		fourK      bool
		threeSixty bool
		hdr        bool
		vr180      bool
		enqueue    bool
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search with the full filter grid",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			query := strings.Join(args, " ")
			filters := youtube.SearchFilters{
				Sort:           sortBy,
				Type:           typ,
				Duration:       duration,
				UploadDate:     uploadDate,
				HD:             hd,
				CC:             cc,
				CreativeCommon: creative,
				Live:           live,
				FourK:          fourK,
				ThreeSixty:     threeSixty,
				HDR:            hdr,
				VR180:          vr180,
			}
			store, err := app.Store()
			if err != nil {
				return err
			}
			if enqueue {
				if store == nil {
					return usageErr("--enqueue needs --db")
				}
			}
			var n int
			err = app.Client.Search(ctx, query, filters, app.PageOptions(false), func(v any) error {
				n++
				if enqueue {
					url, entity := searchEnqueueTarget(v)
					if url != "" {
						_ = store.Enqueue(url, entity, 0)
					}
					return nil
				}
				if store != nil {
					persistSearchValue(store, v)
				}
				return app.Out.Emit(anyRow(v))
			})
			if err != nil && err != youtube.ErrStop {
				return err
			}
			if n == 0 {
				return noResults("no results")
			}
			if enqueue {
				app.logf("enqueued %d items", n)
				return nil
			}
			return app.Out.Flush()
		},
	}
	f := cmd.Flags()
	f.StringVar(&typ, "type", "", "video|channel|playlist")
	f.StringVar(&sortBy, "sort", "", "relevance|date|views|rating")
	f.StringVar(&duration, "duration", "", "short|medium|long")
	f.StringVar(&uploadDate, "upload-date", "", "hour|today|week|month|year")
	f.BoolVar(&hd, "hd", false, "HD only")
	f.BoolVar(&cc, "cc", false, "closed captions / subtitles")
	f.BoolVar(&creative, "creative-commons", false, "Creative Commons license")
	f.BoolVar(&live, "live", false, "live only")
	f.BoolVar(&fourK, "4k", false, "4K only")
	f.BoolVar(&threeSixty, "360", false, "360-degree video")
	f.BoolVar(&hdr, "hdr", false, "HDR only")
	f.BoolVar(&vr180, "vr180", false, "VR180 only")
	f.BoolVar(&enqueue, "enqueue", false, "push results into the crawl queue (needs --db)")
	return cmd
}

func persistSearchValue(store *youtube.Store, v any) {
	switch x := v.(type) {
	case youtube.Video:
		_ = store.UpsertVideo(x)
	case youtube.Channel:
		_ = store.UpsertChannel(x)
	case youtube.Playlist:
		_ = store.UpsertPlaylist(x)
	}
}

func searchEnqueueTarget(v any) (url, entity string) {
	switch x := v.(type) {
	case youtube.Video:
		return "https://www.youtube.com/watch?v=" + x.VideoID, youtube.EntityVideo
	case youtube.Channel:
		return x.URL, youtube.EntityChannel
	case youtube.Playlist:
		return x.URL, youtube.EntityPlaylist
	}
	return "", ""
}
