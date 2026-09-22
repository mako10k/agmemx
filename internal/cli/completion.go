package cli

import (
	"fmt"
	"io"
	"strings"
)

// tryCompletion handles `agmemx __completion --bash [words...]`.
// words are the shell words after the program name. Completion never opens
// the memory store or an embedding provider.
func tryCompletion(args []string, stdout io.Writer) (bool, int) {
	if len(args) < 2 || args[0] != "__completion" || args[1] != "--bash" {
		return false, 0
	}
	mode, candidates := bashCandidates(args[2:])
	fmt.Fprintf(stdout, "__agmemx_completion_mode=%s\n", mode)
	for _, candidate := range candidates {
		if candidate == "" || candidate == "__completion" {
			continue
		}
		fmt.Fprintln(stdout, candidate)
	}
	return true, 0
}

func bashCandidates(words []string) (string, []string) {
	current := ""
	prior := words
	if len(words) > 0 {
		current = words[len(words)-1]
		prior = words[:len(words)-1]
	}
	if len(prior) > 0 {
		if mode, values, ok := completionForFlag(prior[len(prior)-1]); ok {
			return mode, values
		}
	}
	command, index, ok := commandAt(prior)
	if !ok {
		if strings.HasPrefix(current, "-") {
			return "plain", globalFlags()
		}
		return "plain", topLevelCandidates()
	}
	rest := prior[index+1:]
	if command == "help" {
		return helpCandidates(rest, current)
	}
	if sub, parent := subcommandName(command); parent && !hasSubcommand(rest) {
		return "plain", parentCandidates(command, sub, current)
	}
	if strings.HasPrefix(current, "-") {
		return "plain", specFlags(specName(command))
	}
	return "plain", nil
}

func completionForFlag(word string) (string, []string, bool) {
	if !strings.HasPrefix(word, "--") || strings.Contains(word, "=") || word == "--help" {
		return "", nil, false
	}
	switch word {
	case "--dir", "--domain":
		return "dir", nil, true
	case "--embed-fixture", "--source":
		return "file", nil, true
	case "--format":
		return "plain", []string{"json", "text"}, true
	case "--embed-provider":
		return "plain", []string{"ollama", "openai", "fixture"}, true
	case "--reason-kind":
		return "plain", []string{"belief", "observation", "text"}, true
	case "--kind":
		return "plain", []string{"next"}, true
	default:
		return "none", nil, true
	}
}

func commandAt(prior []string) (string, int, bool) {
	for i := 0; i < len(prior); {
		word := prior[i]
		if strings.HasPrefix(word, "-") {
			i = advanceFlag(prior, i)
			continue
		}
		return word, i, true
	}
	return "", 0, false
}

func hasSubcommand(rest []string) bool {
	for i := 0; i < len(rest); {
		if strings.HasPrefix(rest[i], "-") {
			i = advanceFlag(rest, i)
			continue
		}
		return true
	}
	return false
}

func positionals(words []string) []string {
	var out []string
	for i := 0; i < len(words); {
		if strings.HasPrefix(words[i], "-") {
			i = advanceFlag(words, i)
			continue
		}
		out = append(out, words[i])
		i++
	}
	return out
}

func advanceFlag(words []string, i int) int {
	word := words[i]
	name, _, inline := strings.Cut(word, "=")
	if strings.HasPrefix(name, "--") && name != "--help" && !inline && i+1 < len(words) {
		return i + 2
	}
	return i + 1
}

func parentCandidates(command, sub, current string) []string {
	if strings.HasPrefix(current, "-") {
		return specFlags(specName(command))
	}
	if command == "belief" {
		return append([]string{sub}, specFlags(specName(command))...)
	}
	if command == "config" {
		return []string{"show", "set", "unset", "path"}
	}
	return []string{sub}
}

func helpCandidates(rest []string, current string) (string, []string) {
	if strings.HasPrefix(current, "-") {
		return "plain", nil
	}
	pos := positionals(rest)
	if len(pos) == 0 {
		return "plain", helpTopics()
	}
	if len(pos) == 1 {
		if sub, ok := subcommandName(pos[0]); ok {
			return "plain", []string{sub}
		}
	}
	return "plain", nil
}

func subcommandName(command string) (string, bool) {
	switch command {
	case "belief", "relation":
		return "add", true
	case "domain":
		return "attach", true
	case "embed":
		return "reindex", true
	case "config":
		return "show", true
	default:
		return "", false
	}
}

func specName(command string) string {
	switch command {
	case "belief", "believe":
		return "belief add"
	case "relation", "relate":
		return "relation add"
	case "domain", "domain-attach":
		return "domain attach"
	case "embed", "reindex":
		return "embed reindex"
	default:
		return helpTopicFor(command)
	}
}

func specFlags(name string) []string {
	for _, spec := range commandRegistry() {
		if spec.name == name {
			out := make([]string, len(spec.flags))
			copy(out, spec.flags)
			return out
		}
	}
	return nil
}

func topLevelCommands() []string {
	seen := map[string]bool{}
	var names []string
	for _, spec := range commandRegistry() {
		first, _, _ := strings.Cut(spec.name, " ")
		if first == "" || first == "__completion" || seen[first] {
			continue
		}
		seen[first] = true
		names = append(names, first)
	}
	for _, alias := range []string{"relate", "domain-attach", "reindex"} {
		if seen[alias] {
			continue
		}
		seen[alias] = true
		names = append(names, alias)
	}
	return names
}

func globalFlags() []string {
	return []string{
		"--dir",
		"--format",
		"--embed-provider",
		"--embed-model",
		"--embed-base-url",
		"--embed-api-key-env",
		"--embed-fixture",
	}
}

func topLevelCandidates() []string {
	commands := topLevelCommands()
	flags := globalFlags()
	out := make([]string, 0, len(commands)+len(flags))
	out = append(out, commands...)
	out = append(out, flags...)
	return out
}

func helpTopics() []string {
	commands := topLevelCommands()
	out := make([]string, 0, len(commands)+2)
	out = append(out, "concepts", "usecases")
	out = append(out, commands...)
	return out
}
