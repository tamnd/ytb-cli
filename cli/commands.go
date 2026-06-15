package cli

import "github.com/tamnd/any-cli/kit"

// registerEscapeHatches attaches the commands that do not fit the emit-records
// shape of a record operation: the streaming-format and transcript readers, the
// sidecar lookups, media download and extraction, YouTube Music, the local
// crawl store and its queue, the Markdown export, and the config utilities. Each
// is a kit.Command that shares the run state through the context.
func registerEscapeHatches(app *kit.App) {
	app.AddCommand(newFormatsCmd())
	app.AddCommand(newTranscriptCmd())
	app.AddCommand(newChaptersCmd())
	app.AddCommand(newSponsorBlockCmd())
	app.AddCommand(newThumbnailCmd())
	app.AddCommand(newDownloadCmd())
	app.AddCommand(newExtractCmd())
	app.AddCommand(newMusicCmd())
	app.AddCommand(newDiscoverCmd())
	app.AddCommand(newSeedCmd())
	app.AddCommand(newCrawlCmd())
	app.AddCommand(newQueueCmd())
	app.AddCommand(newJobsCmd())
	app.AddCommand(newDBCmd())
	app.AddCommand(newExportCmd())
	app.AddCommand(newConfigCmd())
	app.AddCommand(newVersionCmd())
}
