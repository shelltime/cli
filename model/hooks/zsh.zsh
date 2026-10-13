#!/usr/bin/env zsh

# Check if shelltime command exists
if ! command -v shelltime &> /dev/null
then
    echo "Warning: shelltime command not found. Please install it to track shell usage."
else
    shelltime gc
fi

# Create a timestamp for the session when the shell starts
SESSION_ID=$(date +%Y%m%d%H%M%S)

# Set by preexec and consumed by the next precmd. Empty means this prompt
# does not follow a command we tracked (shell startup, or enter on a blank
# line). fc -ln -1 would otherwise repeat the previous history entry.
typeset -g _SHELLTIME_LAST_CMD=""

# Define the preexec function
preexec() {
    local CMD=$1
    # Check if command starts with exit, logout, or reboot
    if [[ $CMD =~ ^(exit|logout|reboot) ]]; then
        return
    fi

    _SHELLTIME_LAST_CMD=$CMD
    shelltime track -s=zsh -id=$SESSION_ID -cmd="$CMD" -p=pre --ppid=$PPID &> /dev/null
}

# Define the postexec function (in zsh, it's called precmd)
precmd() {
    local LAST_RESULT=$?
    local CMD=$_SHELLTIME_LAST_CMD
    _SHELLTIME_LAST_CMD=""
    if [[ -z $CMD ]]; then
        return
    fi
    # Check if command starts with exit, logout, or reboot
    if [[ $CMD =~ ^(exit|logout|reboot) ]]; then
        return
    fi
    shelltime track -s=zsh -id=$SESSION_ID -cmd="$CMD" -p=post -r=$LAST_RESULT --ppid=$PPID &> /dev/null
}
