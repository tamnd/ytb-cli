package cli

import "github.com/tamnd/any-cli/kit"

// escapeHatches are the commands that do not fit the emit-records shape of a
// record operation: the streaming-format and transcript readers, the sidecar
// lookups, media download and extraction, YouTube Music, the graph plane, the
// crawl and its store, the Markdown export, and the config utilities. Each is a
// kit.Command that shares the run state through the context.
//
// It is a function rather than a list of AddCommand calls so a test can walk it.
// TestEveryReadIsServed pairs each read here with the op that serves it under
// `ytb serve` and `ytb mcp`, and fails on one that has neither an op nor a
// written reason to be missing.
func escapeHatches() []kit.Command {
	return []kit.Command{
		newFormatsCmd(),
		newTranscriptCmd(),
		newCaptionsCmd(),
		newChaptersCmd(),
		newSponsorBlockCmd(),
		newThumbnailCmd(),
		newDownloadCmd(),
		newExtractCmd(),
		newMusicCmd(),
		newDiscoverCmd(),
		newEdgesCmd(),
		newGraphCmd(),
		newPredicatesCmd(),
		newSurfacesCmd(),
		newClientsCmd(),
		newRoutesCmd(),
		newRDFCmd(),
		newCrawlCmd(),
		newArchiveCmd(),
		newDBCmd(),
		newCacheCmd(),
		newQueryCmd(),
		newExportCmd(),
		newConfigCmd(),
		newAuthCmd(),
		newVersionCmd(),
	}
}

// registerEscapeHatches attaches them to the app.
func registerEscapeHatches(app *kit.App) {
	for _, cmd := range escapeHatches() {
		app.AddCommand(cmd)
	}
}
