package moot

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scriptSender runs a real `post --json` through the script at path (as the folder
// dir's cwd) and returns the JSON's sender, or the script's failure.
func scriptSender(t *testing.T, script, dir string) (string, error) {
	t.Helper()
	cmd := exec.Command(script, "post", "--json", "--room", "r", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("name parity\n")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", &ScriptError{Message: strings.TrimSpace(errb.String())}
	}
	var answer struct{ Sender string }
	if err := json.Unmarshal(out.Bytes(), &answer); err != nil {
		t.Fatalf("script output %q: %v", out.String(), err)
	}
	return answer.Sender, nil
}

type nameCase struct {
	name    string
	folder  string            // folder name (default "proj")
	files   map[string]string // relative to the folder
	env     *string           // MOOT_NAME; nil means unset
	want    string            // the sender the script is expected to give
	wantErr bool              // the script is expected to refuse
}

func TestNameParityWithPinnedScript(t *testing.T) {
	cases := []nameCase{
		{name: "folder name", want: "proj"},
		{name: "folder name with dots and dashes", folder: "easy-moot_1.2", want: "easy-moot_1.2"},
		{name: "env wins over .moot/name", env: str("carol"), files: map[string]string{".moot/name": "bob\n"}, want: "carol"},
		{name: "empty MOOT_NAME is skipped", env: str(""), want: "proj"},
		{name: "empty MOOT_NAME falls to .moot/name", env: str(""), files: map[string]string{".moot/name": "bob\n"}, want: "bob"},
		{name: ".moot/name first line only", files: map[string]string{".moot/name": "bob\nsecond\n"}, want: "bob"},
		{name: ".moot/name whitespace removed", files: map[string]string{".moot/name": "  b o\tb \nx"}, want: "bob"},
		{name: ".moot/name without newline", files: map[string]string{".moot/name": "bob"}, want: "bob"},
		{name: ".moot/name CRLF", files: map[string]string{".moot/name": "bob\r\n"}, want: "bob"},
		{name: ".moot/name blank first line falls through, not to line 2", files: map[string]string{".moot/name": "  \nbob\n"}, want: "proj"},
		{name: ".moot/name empty file falls through", files: map[string]string{".moot/name": ""}, want: "proj"},
		{name: ".moot/name blank falls to .gremlin/name", files: map[string]string{".moot/name": "\n", ".gremlin/name": "name=alice\n"}, want: "alice"},
		{name: ".moot/name wins over .gremlin/name", files: map[string]string{".moot/name": "bob\n", ".gremlin/name": "name=alice\n"}, want: "bob"},
		{name: ".gremlin/name name= field", files: map[string]string{".gremlin/name": "name=alice\nsource=generated\n"}, want: "alice"},
		{name: ".gremlin/name name= on a later line", files: map[string]string{".gremlin/name": "source=generated\nname=alice\n"}, want: "alice"},
		{name: ".gremlin/name first of two name= lines", files: map[string]string{".gremlin/name": "name=alice\nname=dave\n"}, want: "alice"},
		{name: ".gremlin/name bare name", files: map[string]string{".gremlin/name": "alice\n"}, want: "alice"},
		{name: ".gremlin/name whitespace in value", files: map[string]string{".gremlin/name": "name= al ice \n"}, want: "alice"},
		{name: ".gremlin/name empty file falls through", files: map[string]string{".gremlin/name": ""}, want: "proj"},
		{name: ".gremlin/name empty name= first line dies", files: map[string]string{".gremlin/name": "name=\n"}, wantErr: true},
		{name: ".gremlin/name empty name= then a real one dies", files: map[string]string{".gremlin/name": "name=\nname=bob\n"}, wantErr: true},
		{name: ".gremlin/name blank name= after another line dies", files: map[string]string{".gremlin/name": "source=x\nname=  \n"}, wantErr: true},
		{name: "unsafe MOOT_NAME dies, no fall-through", env: str("a b"), files: map[string]string{".moot/name": "bob\n"}, wantErr: true},
		{name: "dotted MOOT_NAME dies", env: str(".x"), wantErr: true},
		{name: "unsafe .moot/name dies, no fall-through", files: map[string]string{".moot/name": "a/b\n", ".gremlin/name": "name=alice\n"}, wantErr: true},
		{name: "colon in .moot/name dies", files: map[string]string{".moot/name": "a:b\n"}, wantErr: true},
		{name: "non-ASCII .moot/name dies", files: map[string]string{".moot/name": "bö\n"}, wantErr: true},
		{name: "unsafe .gremlin/name dies", files: map[string]string{".gremlin/name": "name=.hid\n"}, wantErr: true},
		{name: "slash in .gremlin/name dies", files: map[string]string{".gremlin/name": "name=a/b\n"}, wantErr: true},
		{name: "unsafe folder name dies", folder: "bad name", wantErr: true},
		{name: "hidden-style folder name dies", folder: ".proj", wantErr: true},
		{name: "unsafe folder name rescued by .moot/name", folder: "bad name", files: map[string]string{".moot/name": "bob\n"}, want: "bob"},
		{name: "leading dash is safe", files: map[string]string{".moot/name": "-x\n"}, want: "-x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setMootName(t, tc.env)
			folder := tc.folder
			if folder == "" {
				folder = "proj"
			}
			f := newFixture(t, folder)
			f.room("r")
			for rel, content := range tc.files {
				f.write(rel, content)
			}

			sent, scriptErr := scriptSender(t, f.dir+"/moot.sh", f.folder)
			name, goErr := f.open().Name()

			if tc.wantErr {
				if scriptErr == nil {
					t.Fatalf("expectation wrong: the script posted as %q", sent)
				}
				if goErr == nil {
					t.Fatalf("script refuses (%v) but Go resolved %q", scriptErr, name)
				}
				if !strings.Contains(goErr.Error(), "not a filename-safe") && !strings.Contains(goErr.Error(), "not filename-safe") {
					t.Errorf("error does not say why: %v", goErr)
				}
				return
			}
			if scriptErr != nil {
				t.Fatalf("expectation wrong: the script refused: %v", scriptErr)
			}
			if sent != tc.want {
				t.Fatalf("expectation wrong: the script posted as %q, case wants %q", sent, tc.want)
			}
			if goErr != nil {
				t.Fatalf("script posts as %q but Go failed: %v", sent, goErr)
			}
			if name != sent {
				t.Errorf("Go resolved %q, the script's post went out as %q", name, sent)
			}
		})
	}
}

func TestNameParityThroughSymlinkedFolder(t *testing.T) {
	setMootName(t, nil)
	f := newFixture(t, "real-name")
	f.room("r")
	link := filepath.Join(f.root, "link-name")
	if err := os.Symlink(f.folder, link); err != nil {
		t.Fatal(err)
	}
	m, err := Open(link)
	if err != nil {
		t.Fatal(err)
	}

	// Folder-name step: the script names the sender after the link it was run through.
	sent, err := scriptSender(t, link+"/.moot/moot.sh", link)
	if err != nil {
		t.Fatal(err)
	}
	if sent != "link-name" {
		t.Fatalf("expectation wrong: script posted as %q through the link", sent)
	}
	if name, err := m.Name(); err != nil || name != sent {
		t.Errorf("Go %q (%v), script %q", name, err, sent)
	}

	// A .gremlin/name beside the real folder is found through the link.
	f.write(".gremlin/name", "name=alice\n")
	sent, err = scriptSender(t, link+"/.moot/moot.sh", link)
	if err != nil {
		t.Fatal(err)
	}
	if name, err := m.Name(); err != nil || name != sent || name != "alice" {
		t.Errorf("Go %q (%v), script %q", name, err, sent)
	}
}

func TestNameParityWhenFolderIsNotTheWorkingDirectory(t *testing.T) {
	// The script reads its own location, not the cwd; Name never looks at the cwd.
	setMootName(t, nil)
	f := newFixture(t, "proj")
	f.room("r")
	other := t.TempDir()
	sent, err := scriptSender(t, f.dir+"/moot.sh", other)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)
	if name, err := f.open().Name(); err != nil || name != sent || name != "proj" {
		t.Errorf("Go %q (%v), script %q", name, err, sent)
	}
}
