package youtube

import "testing"

func TestRenderOutputTemplate(t *testing.T) {
	f := OutputFields{
		ID:     "dQw4w9WgXcQ",
		Title:  `Rick Astley - Never Gonna Give You Up (Official / "Video")`,
		Author: "Rick Astley",
		Ext:    "mp4",
	}
	got := RenderOutputTemplate("%(title)s [%(id)s].%(ext)s", f)
	want := "Rick Astley - Never Gonna Give You Up (Official Video ) [dQw4w9WgXcQ].mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Path separators written in the template are preserved; unknown fields -> NA.
	got = RenderOutputTemplate("%(uploader)s/%(missing)s/%(id)s.%(ext)s", f)
	want = "Rick Astley/NA/dQw4w9WgXcQ.mp4"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// A slash inside a single field must not create a new path component.
	got = RenderOutputTemplate("%(title)s.%(ext)s", OutputFields{Title: "AC/DC", Ext: "m4a"})
	want = "AC DC.m4a"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
