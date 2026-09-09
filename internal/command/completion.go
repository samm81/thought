package command

import (
	"fmt"
	"io"

	"github.com/samm81/thought/internal/archive"
)

//nolint:dupword // repeated command names are required by the zsh script.
const zshCompletion = `#compdef thought

_thought() {
  local -a commands thoughts

  if (( CURRENT == 2 )); then
    commands=(new edit publish status completion)
    compadd -- "${commands[@]}"
    return
  fi

  case "$words[2]" in
    edit|status|publish)
      if (( CURRENT == 3 )); then
        thoughts=("${(@f)$(command thought __complete "$words[CURRENT]" 2>/dev/null)}")
        if (( ${#thoughts} > 0 )); then
          compadd -- "${thoughts[@]}"
        fi
      fi
      ;;
    completion)
      if (( CURRENT == 3 )); then
        compadd -- zsh
      fi
      ;;
  esac
}

compdef _thought thought
`

func runCompletion(arguments []string, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}

	if len(arguments) != 1 {
		return usageError("completion requires one shell")
	}

	if arguments[0] != "zsh" {
		return usageError("unsupported completion shell %q", arguments[0])
	}

	if _, err := io.WriteString(output, zshCompletion); err != nil {
		return fmt.Errorf("write completion: %w", err)
	}

	return nil
}

func runComplete(arguments []string, output io.Writer) error {
	if err := requireOutput(output); err != nil {
		return err
	}

	if len(arguments) != 1 {
		return usageError("__complete requires one prefix")
	}

	root, err := archive.FromEnvironment()
	if err != nil {
		return err
	}

	names, err := root.ThoughtNames(arguments[0])
	if err != nil {
		return err
	}

	for _, name := range names {
		if _, err := fmt.Fprintln(output, name); err != nil {
			return fmt.Errorf("write completion: %w", err)
		}
	}

	return nil
}
