package editor

import "testing"

func TestBuildVimTabs(t *testing.T) {
	t.Parallel()

	invocation, err := Build("nvim --clean", []string{"01.md", "02.md"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--clean", "-p", "01.md", "02.md"}
	if len(invocation.Args) != len(want) {
		t.Fatalf("args = %#v, want %#v", invocation.Args, want)
	}
	for index := range want {
		if invocation.Args[index] != want[index] {
			t.Fatalf("args = %#v, want %#v", invocation.Args, want)
		}
	}
}

func TestBuildNonVim(t *testing.T) {
	t.Parallel()

	invocation, err := Build("emacs --no-window-system", []string{"01.md"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--no-window-system", "01.md"}
	if len(invocation.Args) != len(want) {
		t.Fatalf("args = %#v, want %#v", invocation.Args, want)
	}
	for index := range want {
		if invocation.Args[index] != want[index] {
			t.Fatalf("args = %#v, want %#v", invocation.Args, want)
		}
	}
}
