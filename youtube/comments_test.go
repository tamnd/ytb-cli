package youtube

import (
	"strings"
	"testing"

	"github.com/tamnd/any-cli/kit/errs"
)

// comments_test.go works off the comment section of a real watch page for
// dQw4w9WgXcQ, fetched from this address, where YouTube turns Restricted Mode on
// whatever is sent. The section is 478 bytes and carries one messageRenderer
// where the comments should be, which is the whole shape.

func TestCommentsRefusalIsQuoted(t *testing.T) {
	section := loadFixture(t, "comments_restricted.json")

	msg := commentsRefusalMessage(section)
	if msg == "" {
		t.Fatal("no refusal found; the section holds a messageRenderer and nothing else")
	}
	// The sentence is quoted whole rather than paraphrased, because it is the only
	// part of this a reader can act on, and because it is localized: matching on
	// the English wording to build our own sentence would go silent in any other
	// language.
	if msg != "Restricted Mode has hidden comments for this video." {
		t.Errorf("message = %q, want YouTube's own sentence", msg)
	}

	// This page carries no comment continuation at all, so a reader that only
	// looked for a token would find none, return zero comments and exit 0. That
	// says the video has no comments. It has millions.
	if token := FindCommentsToken(section); token != "" {
		t.Errorf("found a comment token on a restricted section: %q", token)
	}

	r := newRefusal("comments for dQw4w9WgXcQ", "watch page", msg)
	if !IsRefusal(r) {
		t.Fatal("newRefusal did not build a Refusal")
	}
	// The remedy is attached because this message is one of the measured ones, and
	// it is the half of the answer that says what to do next.
	if !strings.Contains(r.Remedy, "cookies") {
		t.Errorf("no remedy naming cookies: %q", r.Remedy)
	}
}

// TestCommentsRefusalIgnoresOtherMessages guards the scope of the match. A
// messageRenderer is also how YouTube says ordinary things, and one anywhere
// else on a watch page is not a refusal of the comments.
func TestCommentsRefusalIgnoresOtherMessages(t *testing.T) {
	elsewhere := decode(t, `{
	  "itemSectionRenderer": {
	    "sectionIdentifier": "related-items",
	    "contents": [{"messageRenderer": {"text": {"simpleText": "No more results"}}}]
	  }
	}`)
	if msg := commentsRefusalMessage(elsewhere); msg != "" {
		t.Errorf("read %q as a comments refusal; it is the end of the related list", msg)
	}
}

// TestCommentsRefusalExitsFour pins the exit code. Three is "there are none",
// which is what returning an empty list would claim, and it is a different
// statement from YouTube declining to show them.
func TestCommentsRefusalExitsFour(t *testing.T) {
	section := loadFixture(t, "comments_restricted.json")
	err := ExitError(newRefusal("comments for dQw4w9WgXcQ", "watch page", commentsRefusalMessage(section)))
	if err == nil {
		t.Fatal("a refusal mapped to no error")
	}
	if got := errs.KindOf(err); got != errs.KindNeedAuth {
		t.Errorf("kind = %v, want KindNeedAuth, which is exit 4", got)
	}
	// fang prints err.Error()+"." on its own, so a message ending in a period comes
	// out with two.
	if strings.HasSuffix(err.Error(), ".") {
		t.Errorf("message ends in a period, which fang doubles: %q", err.Error())
	}
}
