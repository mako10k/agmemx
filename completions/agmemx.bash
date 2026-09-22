_agmemx_complete()
{
    local cur bin output mode candidates candidate

    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    bin="${AGMEMX_COMPLETION_BIN:-${COMP_WORDS[0]}}"

    if ! output="$("$bin" __completion --bash "${COMP_WORDS[@]:1}" 2>/dev/null)"; then
        return 0
    fi
    mode="${output%%$'\n'*}"
    mode="${mode#__agmemx_completion_mode=}"
    candidates="${output#*$'\n'}"

    case "$mode" in
        file)
            COMPREPLY=( $(compgen -f -- "$cur") )
            return 0
            ;;
        dir)
            COMPREPLY=( $(compgen -d -- "$cur") )
            return 0
            ;;
        none)
            return 0
            ;;
    esac

    while IFS= read -r candidate; do
        if [[ -n $candidate && $candidate == "$cur"* ]]; then
            COMPREPLY+=("$candidate")
        fi
    done <<<"$candidates"
}

complete -F _agmemx_complete agmemx
