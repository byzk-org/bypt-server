package consts

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	bashCmdCompletion = `
# bash completion for bypt                                 -*- shell-script -*-

__bypt_debug()
{
    if [[ -n ${BASH_COMP_DEBUG_FILE} ]]; then
        echo "$*" >> "${BASH_COMP_DEBUG_FILE}"
    fi
}

# Homebrew on Macs have version 1.3 of bash-completion which doesn't include
# _init_completion. This is a very minimal version of that function.
__bypt_init_completion()
{
    COMPREPLY=()
    _get_comp_words_by_ref "$@" cur prev words cword
}

__bypt_index_of_word()
{
    local w word=$1
    shift
    index=0
    for w in "$@"; do
        [[ $w = "$word" ]] && return
        index=$((index+1))
    done
    index=-1
}

__bypt_contains_word()
{
    local w word=$1; shift
    for w in "$@"; do
        [[ $w = "$word" ]] && return
    done
    return 1
}

__bypt_handle_go_custom_completion()
{
    __bypt_debug "${FUNCNAME[0]}: cur is ${cur}, words[*] is ${words[*]}, #words[@] is ${#words[@]}"

    local shellCompDirectiveError=1
    local shellCompDirectiveNoSpace=2
    local shellCompDirectiveNoFileComp=4
    local shellCompDirectiveFilterFileExt=8
    local shellCompDirectiveFilterDirs=16

    local out requestComp lastParam lastChar comp directive args

    # Prepare the command to request completions for the program.
    # Calling ${words[0]} instead of directly bypt allows to handle aliases
    args=("${words[@]:1}")
    requestComp="${words[0]} __completeNoDesc ${args[*]}"

    lastParam=${words[$((${#words[@]}-1))]}
    lastChar=${lastParam:$((${#lastParam}-1)):1}
    __bypt_debug "${FUNCNAME[0]}: lastParam ${lastParam}, lastChar ${lastChar}"

    if [ -z "${cur}" ] && [ "${lastChar}" != "=" ]; then
        # If the last parameter is complete (there is a space following it)
        # We add an extra empty parameter so we can indicate this to the go method.
        __bypt_debug "${FUNCNAME[0]}: Adding extra empty parameter"
        requestComp="${requestComp} \"\""
    fi

    __bypt_debug "${FUNCNAME[0]}: calling ${requestComp}"
    # Use eval to handle any environment variables and such
    out=$(eval "${requestComp}" 2>/dev/null)

    # Extract the directive integer at the very end of the output following a colon (:)
    directive=${out##*:}
    # Remove the directive
    out=${out%:*}
    if [ "${directive}" = "${out}" ]; then
        # There is not directive specified
        directive=0
    fi
    __bypt_debug "${FUNCNAME[0]}: the completion directive is: ${directive}"
    __bypt_debug "${FUNCNAME[0]}: the completions are: ${out[*]}"

    if [ $((directive & shellCompDirectiveError)) -ne 0 ]; then
        # Error code.  No completion.
        __bypt_debug "${FUNCNAME[0]}: received error from custom completion go code"
        return
    else
        if [ $((directive & shellCompDirectiveNoSpace)) -ne 0 ]; then
            if [[ $(type -t compopt) = "builtin" ]]; then
                __bypt_debug "${FUNCNAME[0]}: activating no space"
                compopt -o nospace
            fi
        fi
        if [ $((directive & shellCompDirectiveNoFileComp)) -ne 0 ]; then
            if [[ $(type -t compopt) = "builtin" ]]; then
                __bypt_debug "${FUNCNAME[0]}: activating no file completion"
                compopt +o default
            fi
        fi
    fi

    if [ $((directive & shellCompDirectiveFilterFileExt)) -ne 0 ]; then
        # File extension filtering
        local fullFilter filter filteringCmd
        # Do not use quotes around the $out variable or else newline
        # characters will be kept.
        for filter in ${out[*]}; do
            fullFilter+="$filter|"
        done

        filteringCmd="_filedir $fullFilter"
        __bypt_debug "File filtering command: $filteringCmd"
        $filteringCmd
    elif [ $((directive & shellCompDirectiveFilterDirs)) -ne 0 ]; then
        # File completion for directories only
        local subDir
        # Use printf to strip any trailing newline
        subdir=$(printf "%s" "${out[0]}")
        if [ -n "$subdir" ]; then
            __bypt_debug "Listing directories in $subdir"
            __bypt_handle_subdirs_in_dir_flag "$subdir"
        else
            __bypt_debug "Listing directories in ."
            _filedir -d
        fi
    else
        while IFS='' read -r comp; do
            COMPREPLY+=("$comp")
        done < <(compgen -W "${out[*]}" -- "$cur")
    fi
}

__bypt_handle_reply()
{
    __bypt_debug "${FUNCNAME[0]}"
    local comp
    case $cur in
        -*)
            if [[ $(type -t compopt) = "builtin" ]]; then
                compopt -o nospace
            fi
            local allflags
            if [ ${#must_have_one_flag[@]} -ne 0 ]; then
                allflags=("${must_have_one_flag[@]}")
            else
                allflags=("${flags[*]} ${two_word_flags[*]}")
            fi
            while IFS='' read -r comp; do
                COMPREPLY+=("$comp")
            done < <(compgen -W "${allflags[*]}" -- "$cur")
            if [[ $(type -t compopt) = "builtin" ]]; then
                [[ "${COMPREPLY[0]}" == *= ]] || compopt +o nospace
            fi

            # complete after --flag=abc
            if [[ $cur == *=* ]]; then
                if [[ $(type -t compopt) = "builtin" ]]; then
                    compopt +o nospace
                fi

                local index flag
                flag="${cur%=*}"
                __bypt_index_of_word "${flag}" "${flags_with_completion[@]}"
                COMPREPLY=()
                if [[ ${index} -ge 0 ]]; then
                    PREFIX=""
                    cur="${cur#*=}"
                    ${flags_completion[${index}]}
                    if [ -n "${ZSH_VERSION}" ]; then
                        # zsh completion needs --flag= prefix
                        eval "COMPREPLY=( \"\${COMPREPLY[@]/#/${flag}=}\" )"
                    fi
                fi
            fi
            return 0;
            ;;
    esac

    # check if we are handling a flag with special work handling
    local index
    __bypt_index_of_word "${prev}" "${flags_with_completion[@]}"
    if [[ ${index} -ge 0 ]]; then
        ${flags_completion[${index}]}
        return
    fi

    # we are parsing a flag and don't have a special handler, no completion
    if [[ ${cur} != "${words[cword]}" ]]; then
        return
    fi

    local completions
    completions=("${commands[@]}")
    if [[ ${#must_have_one_noun[@]} -ne 0 ]]; then
        completions+=("${must_have_one_noun[@]}")
    elif [[ -n "${has_completion_function}" ]]; then
        # if a go completion function is provided, defer to that function
        __bypt_handle_go_custom_completion
    fi
    if [[ ${#must_have_one_flag[@]} -ne 0 ]]; then
        completions+=("${must_have_one_flag[@]}")
    fi
    while IFS='' read -r comp; do
        COMPREPLY+=("$comp")
    done < <(compgen -W "${completions[*]}" -- "$cur")

    if [[ ${#COMPREPLY[@]} -eq 0 && ${#noun_aliases[@]} -gt 0 && ${#must_have_one_noun[@]} -ne 0 ]]; then
        while IFS='' read -r comp; do
            COMPREPLY+=("$comp")
        done < <(compgen -W "${noun_aliases[*]}" -- "$cur")
    fi

    if [[ ${#COMPREPLY[@]} -eq 0 ]]; then
		if declare -F __bypt_custom_func >/dev/null; then
			# try command name qualified custom func
			__bypt_custom_func
		else
			# otherwise fall back to unqualified for compatibility
			declare -F __custom_func >/dev/null && __custom_func
		fi
    fi

    # available in bash-completion >= 2, not always present on macOS
    if declare -F __ltrim_colon_completions >/dev/null; then
        __ltrim_colon_completions "$cur"
    fi

    # If there is only 1 completion and it is a flag with an = it will be completed
    # but we don't want a space after the =
    if [[ "${#COMPREPLY[@]}" -eq "1" ]] && [[ $(type -t compopt) = "builtin" ]] && [[ "${COMPREPLY[0]}" == --*= ]]; then
       compopt -o nospace
    fi
}

# The arguments should be in the form "ext1|ext2|extn"
__bypt_handle_filename_extension_flag()
{
    local ext="$1"
    _filedir "@(${ext})"
}

__bypt_handle_subdirs_in_dir_flag()
{
    local dir="$1"
    pushd "${dir}" >/dev/null 2>&1 && _filedir -d && popd >/dev/null 2>&1 || return
}

__bypt_handle_flag()
{
    __bypt_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    # if a command required a flag, and we found it, unset must_have_one_flag()
    local flagname=${words[c]}
    local flagvalue
    # if the word contained an =
    if [[ ${words[c]} == *"="* ]]; then
        flagvalue=${flagname#*=} # take in as flagvalue after the =
        flagname=${flagname%=*} # strip everything after the =
        flagname="${flagname}=" # but put the = back
    fi
    __bypt_debug "${FUNCNAME[0]}: looking for ${flagname}"
    if __bypt_contains_word "${flagname}" "${must_have_one_flag[@]}"; then
        must_have_one_flag=()
    fi

    # if you set a flag which only applies to this command, don't show subcommands
    if __bypt_contains_word "${flagname}" "${local_nonpersistent_flags[@]}"; then
      commands=()
    fi

    # keep flag value with flagname as flaghash
    # flaghash variable is an associative array which is only supported in bash > 3.
    if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
        if [ -n "${flagvalue}" ] ; then
            flaghash[${flagname}]=${flagvalue}
        elif [ -n "${words[ $((c+1)) ]}" ] ; then
            flaghash[${flagname}]=${words[ $((c+1)) ]}
        else
            flaghash[${flagname}]="true" # pad "true" for bool flag
        fi
    fi

    # skip the argument to a two word flag
    if [[ ${words[c]} != *"="* ]] && __bypt_contains_word "${words[c]}" "${two_word_flags[@]}"; then
			  __bypt_debug "${FUNCNAME[0]}: found a flag ${words[c]}, skip the next argument"
        c=$((c+1))
        # if we are looking for a flags value, don't show commands
        if [[ $c -eq $cword ]]; then
            commands=()
        fi
    fi

    c=$((c+1))

}

__bypt_handle_noun()
{
    __bypt_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    if __bypt_contains_word "${words[c]}" "${must_have_one_noun[@]}"; then
        must_have_one_noun=()
    elif __bypt_contains_word "${words[c]}" "${noun_aliases[@]}"; then
        must_have_one_noun=()
    fi

    nouns+=("${words[c]}")
    c=$((c+1))
}

__bypt_handle_command()
{
    __bypt_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"

    local next_command
    if [[ -n ${last_command} ]]; then
        next_command="_${last_command}_${words[c]//:/__}"
    else
        if [[ $c -eq 0 ]]; then
            next_command="_bypt_root_command"
        else
            next_command="_${words[c]//:/__}"
        fi
    fi
    c=$((c+1))
    __bypt_debug "${FUNCNAME[0]}: looking for ${next_command}"
    declare -F "$next_command" >/dev/null && $next_command
}

__bypt_handle_word()
{
    if [[ $c -ge $cword ]]; then
        __bypt_handle_reply
        return
    fi
    __bypt_debug "${FUNCNAME[0]}: c is $c words[c] is ${words[c]}"
    if [[ "${words[c]}" == -* ]]; then
        __bypt_handle_flag
    elif __bypt_contains_word "${words[c]}" "${commands[@]}"; then
        __bypt_handle_command
    elif [[ $c -eq 0 ]]; then
        __bypt_handle_command
    elif __bypt_contains_word "${words[c]}" "${command_aliases[@]}"; then
        # aliashash variable is an associative array which is only supported in bash > 3.
        if [[ -z "${BASH_VERSION}" || "${BASH_VERSINFO[0]}" -gt 3 ]]; then
            words[c]=${aliashash[${words[c]}]}
            __bypt_handle_command
        else
            __bypt_handle_noun
        fi
    else
        __bypt_handle_noun
    fi
    __bypt_handle_word
}

_bypt_completion()
{
    last_command="bypt_completion"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--help")
    flags+=("-h")
    local_nonpersistent_flags+=("--help")
    local_nonpersistent_flags+=("-h")

    must_have_one_flag=()
    must_have_one_noun=()
    must_have_one_noun+=("bash")
    must_have_one_noun+=("fish")
    must_have_one_noun+=("powershell")
    must_have_one_noun+=("zsh")
    noun_aliases=()
}

_bypt_config_ls()
{
    last_command="bypt_config_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_config()
{
    last_command="bypt_config"

    command_aliases=()

    commands=()
    commands+=("ls")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--write=")
    two_word_flags+=("--write")
    two_word_flags+=("-w")
    local_nonpersistent_flags+=("--write")
    local_nonpersistent_flags+=("--write=")
    local_nonpersistent_flags+=("-w")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_export()
{
    last_command="bypt_export"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_help()
{
    last_command="bypt_help"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    has_completion_function=1
    noun_aliases=()
}

_bypt_import()
{
    last_command="bypt_import"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_info()
{
    last_command="bypt_info"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_jdk_ls()
{
    last_command="bypt_jdk_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_jdk_rename()
{
    last_command="bypt_jdk_rename"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_jdk_rm()
{
    last_command="bypt_jdk_rm"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--all")
    flags+=("-a")
    local_nonpersistent_flags+=("--all")
    local_nonpersistent_flags+=("-a")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_jdk()
{
    last_command="bypt_jdk"

    command_aliases=()

    commands=()
    commands+=("ls")
    commands+=("rename")
    commands+=("rm")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_logs()
{
    last_command="bypt_logs"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--clear")
    flags+=("-c")
    local_nonpersistent_flags+=("--clear")
    local_nonpersistent_flags+=("-c")
    flags+=("--endTime=")
    two_word_flags+=("--endTime")
    two_word_flags+=("-t")
    local_nonpersistent_flags+=("--endTime")
    local_nonpersistent_flags+=("--endTime=")
    local_nonpersistent_flags+=("-t")
    flags+=("--export=")
    two_word_flags+=("--export")
    two_word_flags+=("-e")
    local_nonpersistent_flags+=("--export")
    local_nonpersistent_flags+=("--export=")
    local_nonpersistent_flags+=("-e")
    flags+=("--follow")
    flags+=("-f")
    local_nonpersistent_flags+=("--follow")
    local_nonpersistent_flags+=("-f")
    flags+=("--startTime=")
    two_word_flags+=("--startTime")
    two_word_flags+=("-s")
    local_nonpersistent_flags+=("--startTime")
    local_nonpersistent_flags+=("--startTime=")
    local_nonpersistent_flags+=("-s")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_ls()
{
    last_command="bypt_ls"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_ps()
{
    last_command="bypt_ps"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_restart()
{
    last_command="bypt_restart"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--withStartConfig=")
    two_word_flags+=("--withStartConfig")
    two_word_flags+=("-w")
    local_nonpersistent_flags+=("--withStartConfig")
    local_nonpersistent_flags+=("--withStartConfig=")
    local_nonpersistent_flags+=("-w")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_rm()
{
    last_command="bypt_rm"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--all")
    flags+=("-a")
    local_nonpersistent_flags+=("--all")
    local_nonpersistent_flags+=("-a")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_start()
{
    last_command="bypt_start"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--withStartConfig=")
    two_word_flags+=("--withStartConfig")
    two_word_flags+=("-w")
    local_nonpersistent_flags+=("--withStartConfig")
    local_nonpersistent_flags+=("--withStartConfig=")
    local_nonpersistent_flags+=("-w")
    flags+=("--currentVersion=")
    two_word_flags+=("--currentVersion")
    two_word_flags+=("-v")
    local_nonpersistent_flags+=("--currentVersion")
    local_nonpersistent_flags+=("--currentVersion=")
    local_nonpersistent_flags+=("-v")
    flags+=("--copyFile=")
    two_word_flags+=("--copyFile")
    two_word_flags+=("-f")
    local_nonpersistent_flags+=("--copyFile")
    local_nonpersistent_flags+=("--copyFile=")
    local_nonpersistent_flags+=("-f")
    flags+=("--envConfig=")
    two_word_flags+=("--envConfig")
    two_word_flags+=("-e")
    local_nonpersistent_flags+=("--envConfig")
    local_nonpersistent_flags+=("--envConfig=")
    local_nonpersistent_flags+=("-e")
    flags+=("--javaPackName=")
    two_word_flags+=("--javaPackName")
    two_word_flags+=("-p")
    local_nonpersistent_flags+=("--javaPackName")
    local_nonpersistent_flags+=("--javaPackName=")
    local_nonpersistent_flags+=("-p")
    flags+=("--javaCmdPath=")
    two_word_flags+=("--javaCmdPath")
    two_word_flags+=("-c")
    local_nonpersistent_flags+=("--javaCmdPath")
    local_nonpersistent_flags+=("--javaCmdPath=")
    local_nonpersistent_flags+=("-c")
    flags+=("--saveAppSuffix")
    flags+=("-s")
    local_nonpersistent_flags+=("--saveAppSuffix")
    local_nonpersistent_flags+=("-s")
    flags+=("--restart=")
    two_word_flags+=("--restart")
    two_word_flags+=("-r")
    local_nonpersistent_flags+=("--restart")
    local_nonpersistent_flags+=("--restart=")
    local_nonpersistent_flags+=("-r")
    flags+=("--pluginEnvConfig=")
    two_word_flags+=("--pluginEnvConfig")
    local_nonpersistent_flags+=("--pluginEnvConfig")
    local_nonpersistent_flags+=("--pluginEnvConfig=")
    flags+=("--Xmx=")
    two_word_flags+=("--Xmx")
    local_nonpersistent_flags+=("--Xmx")
    local_nonpersistent_flags+=("--Xmx=")
    flags+=("--Xms=")
    two_word_flags+=("--Xms")
    local_nonpersistent_flags+=("--Xms")
    local_nonpersistent_flags+=("--Xms=")
    flags+=("--Xmn=")
    two_word_flags+=("--Xmn")
    local_nonpersistent_flags+=("--Xmn")
    local_nonpersistent_flags+=("--Xmn=")
    flags+=("--PermSize=")
    two_word_flags+=("--PermSize")
    local_nonpersistent_flags+=("--PermSize")
    local_nonpersistent_flags+=("--PermSize=")
    flags+=("--MaxPermSize=")
    two_word_flags+=("--MaxPermSize")
    local_nonpersistent_flags+=("--MaxPermSize")
    local_nonpersistent_flags+=("--MaxPermSize=")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_stop()
{
    last_command="bypt_stop"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--withStartConfig=")
    two_word_flags+=("--withStartConfig")
    two_word_flags+=("-w")
    local_nonpersistent_flags+=("--withStartConfig")
    local_nonpersistent_flags+=("--withStartConfig=")
    local_nonpersistent_flags+=("-w")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_sync()
{
    last_command="bypt_sync"

    command_aliases=()

    commands=()

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()

    flags+=("--all")
    flags+=("-a")
    local_nonpersistent_flags+=("--all")
    local_nonpersistent_flags+=("-a")
    flags+=("--app")
    flags+=("-m")
    local_nonpersistent_flags+=("--app")
    local_nonpersistent_flags+=("-m")
    flags+=("--jdk")
    flags+=("-j")
    local_nonpersistent_flags+=("--jdk")
    local_nonpersistent_flags+=("-j")
    flags+=("--version")
    flags+=("-v")
    local_nonpersistent_flags+=("--version")
    local_nonpersistent_flags+=("-v")

    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

_bypt_root_command()
{
    last_command="bypt"

    command_aliases=()

    commands=()
    commands+=("completion")
    commands+=("config")
    commands+=("export")
    commands+=("help")
    commands+=("import")
    commands+=("info")
    commands+=("jdk")
    commands+=("logs")
    commands+=("ls")
    commands+=("ps")
    commands+=("restart")
    commands+=("rm")
    commands+=("start")
    commands+=("stop")
    commands+=("sync")

    flags=()
    two_word_flags=()
    local_nonpersistent_flags=()
    flags_with_completion=()
    flags_completion=()


    must_have_one_flag=()
    must_have_one_noun=()
    noun_aliases=()
}

__start_bypt()
{
    local cur prev words cword
    declare -A flaghash 2>/dev/null || :
    declare -A aliashash 2>/dev/null || :
    if declare -F _init_completion >/dev/null 2>&1; then
        _init_completion -s || return
    else
        __bypt_init_completion -n "=" || return
    fi

    local c=0
    local flags=()
    local two_word_flags=()
    local local_nonpersistent_flags=()
    local flags_with_completion=()
    local flags_completion=()
    local commands=("bypt")
    local must_have_one_flag=()
    local must_have_one_noun=()
    local has_completion_function
    local last_command
    local nouns=()

    __bypt_handle_word
}

if [[ $(type -t compopt) = "builtin" ]]; then
    complete -o default -F __start_bypt bypt
else
    complete -o default -o nospace -F __start_bypt bypt
fi

# ex: ts=4 sw=4 et filetype=sh
`
	zshCmdCompletion = `compdef _bypt bypt
# zsh completion for bypt                                 -*- shell-script -*-

__bypt_debug()
{
    local file="$BASH_COMP_DEBUG_FILE"
    if [[ -n ${file} ]]; then
        echo "$*" >> "${file}"
    fi
}

_bypt()
{
    local shellCompDirectiveError=1
    local shellCompDirectiveNoSpace=2
    local shellCompDirectiveNoFileComp=4
    local shellCompDirectiveFilterFileExt=8
    local shellCompDirectiveFilterDirs=16

    local lastParam lastChar flagPrefix requestComp out directive compCount comp lastComp
    local -a completions

    __bypt_debug "\n========= starting completion logic =========="
    __bypt_debug "CURRENT: ${CURRENT}, words[*]: ${words[*]}"

    # The user could have moved the cursor backwards on the command-line.
    # We need to trigger completion from the $CURRENT location, so we need
    # to truncate the command-line ($words) up to the $CURRENT location.
    # (We cannot use $CURSOR as its value does not work when a command is an alias.)
    words=("${=words[1,CURRENT]}")
    __bypt_debug "Truncated words[*]: ${words[*]},"

    lastParam=${words[-1]}
    lastChar=${lastParam[-1]}
    __bypt_debug "lastParam: ${lastParam}, lastChar: ${lastChar}"

    # For zsh, when completing a flag with an = (e.g., bypt -n=<TAB>)
    # completions must be prefixed with the flag
    setopt local_options BASH_REMATCH
    if [[ "${lastParam}" =~ '-.*=' ]]; then
        # We are dealing with a flag with an =
        flagPrefix="-P ${BASH_REMATCH}"
    fi

    # Prepare the command to obtain completions
    requestComp="${words[1]} __complete ${words[2,-1]}"
    if [ "${lastChar}" = "" ]; then
        # If the last parameter is complete (there is a space following it)
        # We add an extra empty parameter so we can indicate this to the go completion code.
        __bypt_debug "Adding extra empty parameter"
        requestComp="${requestComp} \"\""
    fi

    __bypt_debug "About to call: eval ${requestComp}"

    # Use eval to handle any environment variables and such
    out=$(eval ${requestComp} 2>/dev/null)
    __bypt_debug "completion output: ${out}"

    # Extract the directive integer following a : from the last line
    local lastLine
    while IFS='\n' read -r line; do
        lastLine=${line}
    done < <(printf "%s\n" "${out[@]}")
    __bypt_debug "last line: ${lastLine}"

    if [ "${lastLine[1]}" = : ]; then
        directive=${lastLine[2,-1]}
        # Remove the directive including the : and the newline
        local suffix
        (( suffix=${#lastLine}+2))
        out=${out[1,-$suffix]}
    else
        # There is no directive specified.  Leave $out as is.
        __bypt_debug "No directive found.  Setting do default"
        directive=0
    fi

    __bypt_debug "directive: ${directive}"
    __bypt_debug "completions: ${out}"
    __bypt_debug "flagPrefix: ${flagPrefix}"

    if [ $((directive & shellCompDirectiveError)) -ne 0 ]; then
        __bypt_debug "Completion received error. Ignoring completions."
        return
    fi

    compCount=0
    while IFS='\n' read -r comp; do
        if [ -n "$comp" ]; then
            # If requested, completions are returned with a description.
            # The description is preceded by a TAB character.
            # For zsh's _describe, we need to use a : instead of a TAB.
            # We first need to escape any : as part of the completion itself.
            comp=${comp//:/\\:}

            local tab=$(printf '\t')
            comp=${comp//$tab/:}

            ((compCount++))
            __bypt_debug "Adding completion: ${comp}"
            completions+=${comp}
            lastComp=$comp
        fi
    done < <(printf "%s\n" "${out[@]}")

    if [ $((directive & shellCompDirectiveFilterFileExt)) -ne 0 ]; then
        # File extension filtering
        local filteringCmd
        filteringCmd='_files'
        for filter in ${completions[@]}; do
            if [ ${filter[1]} != '*' ]; then
                # zsh requires a glob pattern to do file filtering
                filter="\*.$filter"
            fi
            filteringCmd+=" -g $filter"
        done
        filteringCmd+=" ${flagPrefix}"

        __bypt_debug "File filtering command: $filteringCmd"
        _arguments '*:filename:'"$filteringCmd"
    elif [ $((directive & shellCompDirectiveFilterDirs)) -ne 0 ]; then
        # File completion for directories only
        local subDir
        subdir="${completions[1]}"
        if [ -n "$subdir" ]; then
            __bypt_debug "Listing directories in $subdir"
            pushd "${subdir}" >/dev/null 2>&1
        else
            __bypt_debug "Listing directories in ."
        fi

        _arguments '*:dirname:_files -/'" ${flagPrefix}"
        if [ -n "$subdir" ]; then
            popd >/dev/null 2>&1
        fi
    elif [ $((directive & shellCompDirectiveNoSpace)) -ne 0 ] && [ ${compCount} -eq 1 ]; then
        __bypt_debug "Activating nospace."
        # We can use compadd here as there is no description when
        # there is only one completion.
        compadd -S '' "${lastComp}"
    elif [ ${compCount} -eq 0 ]; then
        if [ $((directive & shellCompDirectiveNoFileComp)) -ne 0 ]; then
            __bypt_debug "deactivating file completion"
        else
            # Perform file completion
            __bypt_debug "activating file completion"
            _arguments '*:filename:_files'" ${flagPrefix}"
        fi
    else
        _describe "completions" completions $(echo $flagPrefix)
    fi
}

# don't run the completion function when being source-ed or eval-ed
if [ "$funcstack[1]" = "_bypt" ]; then
	_bypt
fi
`
)

func initBashConfig() {
	fileDir := filepath.Join(HomeDir, ".devTools")
	_ = os.MkdirAll(fileDir, 0777)
	writeZshFile(fileDir)
	writeBashFile(fileDir)
}

func writeZshFile(runDir string) {
	zshFile := filepath.Join(runDir, ".bypt_zsh")
	os.RemoveAll(zshFile)
	//stat, err := os.Stat(zshFile)
	//if err == nil && !stat.IsDir() {
	//	return
	//}

	if err := ioutil.WriteFile(zshFile, []byte(zshCmdCompletion), 0666); err != nil {
		fmt.Println("写出zsh自动补全脚本失败")
	}

	file, err := os.OpenFile(filepath.Join(HomeDir, ".zshrc"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Println("配置zsh自动补全脚本失败")
		return
	}
	defer file.Close()

	file.WriteString("\n")
	file.WriteString("source " + zshFile)
}

func writeBashFile(runDir string) {
	//zshFile := filepath.Join(runDir, ".bypt_bash")
	//stat, err := os.Stat(zshFile)
	//if err == nil && !stat.IsDir() {
	//	return
	//}
	//
	//if err = ioutil.WriteFile(zshFile, []byte(zshCmdCompletion), 0666); err != nil {
	//	fmt.Println("写出Bash自动补全脚本失败")
	//}

	_ = os.RemoveAll("/etc/bash_completion.d/bypt_completion")
	if err := ioutil.WriteFile("/etc/bash_completion.d/bypt_completion", []byte(bashCmdCompletion), 0666); err != nil {
		fmt.Println("配置Bash自动补全脚本失败")
	}

}
