package moot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKindOfReadsTheNameOnly(t *testing.T) {
	cases := []struct {
		folder string
		want   Kind
	}{
		{"dev", Kind{}},
		{"dev.archived", Kind{Archived: true}},
		{"dm-alice-bob", Kind{DM: true}},
		{"dm-alice-bob.archived", Kind{Archived: true, DM: true}},
		{"archived", Kind{}},                // the suffix needs its dot
		{"my-dm-room", Kind{}},              // the prefix must begin the name
		{".archived", Kind{Archived: true}}, // a bare suffix still ends with it
	}
	for _, c := range cases {
		if got := KindOf(c.folder, Kept); got != c.want {
			t.Errorf("KindOf(%q) = %+v, want %+v", c.folder, got, c.want)
		}
	}
	if got := KindOf("dev", Heard); got.Standing != Heard {
		t.Errorf("standing lost: %+v", got)
	}
}

func TestRoomsTellKeptHeardArchivedAndDMByName(t *testing.T) {
	f := newFixture(t, "me")
	f.room("dev")
	f.room("old.archived")
	f.room("dm-me-you")
	// A link is heard; its kind comes from the folder it leads to, so a link named
	// plainly that leads to an archived folder is archived (link repair has not
	// renamed it yet).
	f.mkdir("../keeper/.moot/rooms/shared/posts")
	f.mkdir("../keeper/.moot/rooms/gone.archived/posts")
	f.mkdir("../keeper/.moot/rooms/dm-keeper-me/posts")
	for local, target := range map[string]string{
		"shared":    "shared",
		"gone":      "gone.archived",
		"dm-keeper": "dm-keeper-me",
	} {
		if err := os.Symlink(filepath.Join(f.root, "keeper/.moot/rooms", target), f.path(".moot/rooms/"+local)); err != nil {
			t.Fatal(err)
		}
	}
	rooms, err := f.open().Rooms()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Kind{}
	for _, r := range rooms {
		got[r.Name] = r.Kind
	}
	want := map[string]Kind{
		"dev":          {},
		"old.archived": {Archived: true},
		"dm-me-you":    {DM: true},
		"shared":       {Standing: Heard},
		"gone":         {Standing: Heard, Archived: true},
		"dm-keeper":    {Standing: Heard, DM: true},
	}
	if len(got) != len(want) {
		t.Fatalf("rooms %+v", got)
	}
	for name, k := range want {
		if got[name] != k {
			t.Errorf("%s: kind %+v, want %+v", name, got[name], k)
		}
	}
}

func TestDiscoverKindsAndArchivedRoomsAreReadOnly(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	room := func(p string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, p, "posts"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	room("opened/.moot/rooms/live")
	room("opened/.moot/rooms/kept.archived")
	room("opened/.moot/rooms/dm-opened-other")
	room("other/.moot/rooms/theirs")
	room("other/.moot/rooms/theirs-old.archived")
	room("other/.moot/rooms/dm-other-third")
	room("other/.moot/rooms/heard-old.archived")
	if err := os.Symlink(filepath.Join(root, "other/.moot/rooms/heard-old.archived"), filepath.Join(root, "opened/.moot/rooms/heard-old")); err != nil {
		t.Fatal(err)
	}
	d, err := Discover(filepath.Join(root, "opened"), root, 2)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		kind Kind
		post string
	}
	got := map[string]row{}
	for _, r := range d.Rooms {
		got[filepath.Base(r.Keeper)+"/"+r.Name] = row{r.Kind, r.PostName}
	}
	want := map[string]row{
		"opened/live":               {Kind{}, "live"},
		"opened/kept.archived":      {Kind{Archived: true}, ""},
		"opened/dm-opened-other":    {Kind{DM: true}, "dm-opened-other"},
		"other/theirs":              {Kind{Standing: Elsewhere}, ""},
		"other/theirs-old.archived": {Kind{Standing: Elsewhere, Archived: true}, ""},
		"other/dm-other-third":      {Kind{Standing: Elsewhere, DM: true}, ""},
		"other/heard-old.archived":  {Kind{Standing: Heard, Archived: true}, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("rooms %+v", got)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: %+v, want %+v", k, got[k], w)
		}
	}
}
