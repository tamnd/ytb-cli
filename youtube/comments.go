package youtube

import (
	"context"
	"fmt"
)

// comments.go is the comment plane. Doc 05 section 4.
//
// Two things shape it.
//
// The first is Restricted Mode. YouTube turns it on for some datacenter and
// server addresses whatever cookies are sent, and when it is on the comment
// section is replaced by a messageRenderer reading "Restricted Mode has hidden
// comments for this video." Returning zero comments there is a claim the video
// has none, which is a different statement and a false one. So it is a refusal
// carrying YouTube's own sentence, and it exits 4.
//
// The second is where the token lives. The /next API strips the comment
// continuation for an unauthenticated caller, so the watch page's ytInitialData
// is the source, and the visitor id it was minted against has to be sent with
// it. Every token here is found by searching for the key rather than by walking
// a path, because the comment section carries a reply token, a sort chip token
// and a next-page token in the same response, and telling them apart is what
// continuation.go is for.

// StreamComments streams a video's comments, and its replies when asked.
// Returning ErrStop from emit halts iteration cleanly.
func (c *Client) StreamComments(ctx context.Context, idOrURL string, opt CommentOptions, emit func(Comment) error) error {
	emit = stampEmit(c, emit)
	videoID := ExtractVideoID(idOrURL)
	if videoID == "" {
		videoID = idOrURL
	}

	it := NewInnerTube(c)
	data, _, err := c.FetchPageData(ctx, NormalizeVideoURL(idOrURL))
	if err != nil {
		return fmt.Errorf("comments page: %w", err)
	}
	var initial any
	var visitor string
	if data != nil {
		initial = data.InitialData
		visitor = data.VisitorData
	}

	if msg := commentsRefusalMessage(initial); msg != "" {
		return newRefusal("comments for "+videoID, "watch page", msg)
	}

	token := FindCommentsToken(initial)
	if token == "" {
		// Nothing to page. A video with comments turned off says so on the page and
		// carries no token, which is an answer and not a failure.
		return nil
	}

	total := 0
	pages := 0
	for token != "" {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return nil
		}

		resp, err := it.CommentContinuationWEB(ctx, token, visitor)
		if err != nil {
			return fmt.Errorf("comment page %d: %w", pages+1, err)
		}
		// A continuation can refuse too, and it refuses the same way the page does.
		if msg := commentsRefusalMessage(resp); msg != "" {
			return newRefusal("comments for "+videoID, "comment continuation", msg)
		}

		entities := collectCommentEntities(resp)
		batch := 0
		stopped := false

		walkJSON(resp, func(m map[string]any) {
			if stopped {
				return
			}
			ctr, ok := m["commentThreadRenderer"].(map[string]any)
			if !ok {
				return
			}
			comment := commentFromThread(ctr, entities, videoID)
			if comment == nil {
				return
			}
			if opt.Max > 0 && total+batch >= opt.Max {
				return
			}
			if err := emit(*comment); err != nil {
				stopped = true
				return
			}
			batch++
			if opt.Replies && comment.ReplyCount > 0 {
				// A reply thread is its own list under commentRepliesRenderer, which
				// FindContinuationToken is right to skip and this is right to ask for.
				if replyToken := FindContinuationTokenUnder(ctr, "commentRepliesViewModel"); replyToken != "" {
					batch += streamReplies(ctx, it, videoID, comment.ID, visitor, replyToken, opt, total+batch, emit)
				} else if replyToken := FindContinuationTokenUnder(ctr, "commentRepliesRenderer"); replyToken != "" {
					batch += streamReplies(ctx, it, videoID, comment.ID, visitor, replyToken, opt, total+batch, emit)
				}
			}
		})

		total += batch
		pages++
		if stopped {
			return nil
		}
		token = FindContinuationToken(resp)
		if batch == 0 {
			break
		}
	}
	return nil
}

// commentsRefusalMessage returns YouTube's sentence when the comment section was
// replaced by one, and "" when it was not.
//
// The message is quoted rather than matched on, because it is localized and
// because paraphrasing it throws away the only thing the reader can act on. What
// is matched is the section: a messageRenderer standing where the comment items
// should be. Elsewhere on a watch page a messageRenderer says ordinary things.
func commentsRefusalMessage(root any) string {
	msg := ""
	walkJSON(root, func(m map[string]any) {
		if msg != "" {
			return
		}
		isr, ok := m["itemSectionRenderer"].(map[string]any)
		if !ok || stringValue(isr["sectionIdentifier"]) != "comment-item-section" {
			return
		}
		walkJSON(isr, func(mm map[string]any) {
			if msg != "" {
				return
			}
			mr, ok := mm["messageRenderer"].(map[string]any)
			if !ok {
				return
			}
			if text := extractText(mr["text"]); text != "" {
				msg = text
			}
		})
	})
	return msg
}

// commentFromThread resolves a commentThreadRenderer to a Comment, preferring
// the entity payload and falling back to an inline commentRenderer.
func commentFromThread(ctr map[string]any, entities map[string]*Comment, videoID string) *Comment {
	if key := threadCommentKey(ctr); key != "" {
		if c := entities[key]; c != nil {
			clone := *c
			clone.VideoID = videoID
			return &clone
		}
	}
	if commentMap, ok := ctr["comment"].(map[string]any); ok {
		return ParseCommentRenderer(commentMap, videoID, "")
	}
	return nil
}

// threadCommentKey extracts the entity key a commentThreadRenderer references
// through its commentViewModel.
func threadCommentKey(ctr map[string]any) string {
	cvm := mapValue(ctr, "commentViewModel")
	if cvm == nil {
		return ""
	}
	if inner := mapValue(cvm, "commentViewModel"); inner != nil {
		cvm = inner
	}
	return stringValue(cvm["commentKey"])
}

// streamReplies pages one comment's replies and returns how many it emitted.
func streamReplies(
	ctx context.Context,
	it *InnerTubeClient,
	videoID, parentID, visitor, token string,
	opt CommentOptions,
	before int,
	emit func(Comment) error,
) int {
	count := 0
	for token != "" {
		if ctx.Err() != nil {
			return count
		}
		resp, err := it.CommentContinuationWEB(ctx, token, visitor)
		if err != nil {
			return count
		}
		entities := collectCommentEntities(resp)
		emitted := 0
		walkJSON(resp, func(m map[string]any) {
			if cvm, ok := m["commentViewModel"].(map[string]any); ok {
				key := stringValue(cvm["commentKey"])
				if key == "" {
					return
				}
				reply := entities[key]
				if reply == nil {
					return
				}
				if opt.Max > 0 && before+count >= opt.Max {
					return
				}
				clone := *reply
				clone.VideoID = videoID
				clone.ParentID = parentID
				if err := emit(clone); err != nil {
					return
				}
				count++
				emitted++
				return
			}
			if rr, ok := m["commentRenderer"].(map[string]any); ok {
				if stringValue(rr["commentId"]) == "" {
					return
				}
				reply := ParseCommentRenderer(map[string]any{"commentRenderer": rr}, videoID, parentID)
				if reply == nil {
					return
				}
				if opt.Max > 0 && before+count >= opt.Max {
					return
				}
				if err := emit(*reply); err != nil {
					return
				}
				count++
				emitted++
			}
		})
		// A reply page's own next token is under the replies renderer, which is the
		// marker FindContinuationToken skips, so it is asked for by name.
		next := FindContinuationTokenUnder(resp, "continuationItemRenderer")
		if next == token || emitted == 0 {
			return count
		}
		token = next
	}
	return count
}
