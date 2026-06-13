package youtube

import (
	"context"
	"errors"
	"fmt"
)

// ErrCommentsRestricted is returned when a video's comments are hidden by
// Restricted Mode. YouTube applies Restricted Mode to some server and
// datacenter requests regardless of cookies, so callers can present a clear
// message rather than mistaking it for a video with no comments.
var ErrCommentsRestricted = errors.New("comments hidden by Restricted Mode")

// StreamComments streams comments (and optionally replies) for a video.
// idOrURL may be a video ID or any URL form. Returning ErrStop from emit halts
// iteration cleanly.
//
// The watch page's ytInitialData is the reliable source for both the comment
// continuation token and the visitor session it is bound to; the /next API
// strips the token for unauthenticated requests. Comment bodies arrive as
// entity payloads (the modern model), with the classic commentRenderer kept as
// a fallback for replies and older responses.
func (c *Client) StreamComments(ctx context.Context, idOrURL string, opt CommentOptions, emit func(Comment) error) error {
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

	if CommentsRestricted(initial) {
		return ErrCommentsRestricted
	}

	contToken := FindCommentsToken(initial)
	if contToken == "" {
		// Fallback to the /next API token discovery for older response shapes.
		if nextResp, err := it.NextMWEB(ctx, videoID); err == nil {
			contToken = extractCommentContinuationToken(nextResp)
			if contToken == "" {
				contToken = extractCommentContFromNextResp(nextResp)
			}
		}
	}
	if contToken == "" {
		return nil // no comments, or comments are disabled
	}

	total := 0
	pages := 0

	for contToken != "" {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opt.Max > 0 && total >= opt.Max {
			return nil
		}
		if opt.MaxPages > 0 && pages >= opt.MaxPages {
			return nil
		}

		resp, err := it.CommentContinuationWEB(ctx, contToken, visitor)
		if err != nil {
			return fmt.Errorf("comment page %d: %w", pages+1, err)
		}

		entities := collectCommentEntities(resp)
		var nextToken string
		var stopped bool
		batchCount := 0

		walkJSON(resp, func(m map[string]any) {
			if stopped {
				return
			}
			// Modern model: a thread references its body by entity key.
			if ctr, ok := m["commentThreadRenderer"].(map[string]any); ok {
				comment := commentFromThread(ctr, entities, videoID)
				if comment == nil {
					return
				}
				if opt.Max > 0 && total+batchCount >= opt.Max {
					return
				}
				if emitErr := emit(*comment); emitErr != nil {
					stopped = true
					return
				}
				batchCount++
				if opt.Replies && comment.ReplyCount > 0 {
					if replyCont := extractReplyToken(ctr); replyCont != "" {
						batchCount += streamReplies(ctx, it, c, videoID, comment.ID, visitor, replyCont, opt, &total, emit)
					}
				}
				return
			}
			// Continuation token for the next page.
			if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
				if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
					if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
						if tok := stringValue(cmd["token"]); tok != "" && nextToken == "" {
							nextToken = tok
						}
					}
				}
			}
		})

		total += batchCount
		pages++
		if stopped {
			return nil
		}
		contToken = nextToken
		if batchCount == 0 {
			break
		}
	}
	return nil
}

// commentFromThread resolves a commentThreadRenderer to a Comment, preferring
// the entity payload (modern) and falling back to an inline commentRenderer.
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

// streamReplies pages through replies for a comment and emits each one.
// Returns the number of replies emitted.
func streamReplies(
	ctx context.Context,
	it *InnerTubeClient,
	c *Client,
	videoID, parentID, visitor, contToken string,
	opt CommentOptions,
	total *int,
	emit func(Comment) error,
) int {
	count := 0
	for contToken != "" {
		if ctx.Err() != nil {
			return count
		}
		_ = c // keep reference for future rate-limit use
		resp, err := it.CommentContinuationWEB(ctx, contToken, visitor)
		if err != nil {
			return count
		}
		entities := collectCommentEntities(resp)
		var nextToken string
		walkJSON(resp, func(m map[string]any) {
			// Modern reply: a commentViewModel referencing an entity key.
			if cvm, ok := m["commentViewModel"].(map[string]any); ok {
				if key := stringValue(cvm["commentKey"]); key != "" {
					if c := entities[key]; c != nil {
						if opt.Max > 0 && *total+count >= opt.Max {
							return
						}
						clone := *c
						clone.VideoID = videoID
						clone.ParentID = parentID
						if emitErr := emit(clone); emitErr != nil {
							return
						}
						count++
						return
					}
				}
			}
			// Legacy reply renderer.
			if rr, ok := m["commentRenderer"].(map[string]any); ok {
				if stringValue(rr["commentId"]) == "" {
					return
				}
				comment := ParseCommentRenderer(map[string]any{"commentRenderer": rr}, videoID, parentID)
				if comment == nil {
					return
				}
				if opt.Max > 0 && *total+count >= opt.Max {
					return
				}
				if emitErr := emit(*comment); emitErr != nil {
					return
				}
				count++
			}
			if cir, ok := m["continuationItemRenderer"].(map[string]any); ok {
				if ep := mapValue(cir, "continuationEndpoint"); ep != nil {
					if cmd := mapValue(ep, "continuationCommand"); cmd != nil {
						if tok := stringValue(cmd["token"]); tok != "" && nextToken == "" {
							nextToken = tok
						}
					}
				}
			}
		})
		contToken = nextToken
	}
	return count
}
