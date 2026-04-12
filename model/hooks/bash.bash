#!/bin/bash

# Source bash-preexec.sh if it exists
if [ -f "bash-preexec.sh" ]; then
    source "bash-preexec.sh"
else
    # Attempt to find bash-preexec.sh in the same directory as this script
    _SHELLTIME_HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    if [ -f "$_SHELLTIME_HOOK_DIR/bash-preexec.sh" ]; then
        source "$_SHELLTIME_HOOK_DIR/bash-preexec.sh"
    else
        echo "Warning: bash-preexec.sh not found. Pre-execution hooks will not work."
        return 1
    fi
fi

# Check if shelltime CLI exists
if ! command -v shelltime &> /dev/null
then
    echo "Warning: shelltime CLI not found. Please install it to enable time tracking."
else
    shelltime gc
fi

# Create a timestamp for the session when the shell starts
SESSION_ID=$(date +%Y%m%d%H%M%S)
LAST_COMMAND=""

# Function to be executed before each command
preexec_invoke_cmd() {
    local CMD="$1"
    LAST_COMMAND="$CMD"
    # Check if command starts with exit, logout, or reboot
    if [[ "$CMD" =~ ^(exit|logout|reboot) ]]; then
        return
    fi

    # Avoid tracking shelltime commands themselves to prevent loops
    if [[ "$CMD" =~ ^shelltime ]]; then
        return
    fi

    shelltime track -s=bash -id=$SESSION_ID -cmd="$CMD" -p=pre --ppid=$PPID &> /dev/null
}

# Function to be executed after each command (before prompt)
precmd_invoke_cmd() {
    local LAST_RESULT=$?
    # BASH_COMMAND in precmd is the *previous* command
    local CMD="$LAST_COMMAND"
    # Check if command starts with exit, logout, or reboot
    if [[ "$CMD" =~ ^(exit|logout|reboot) ]]; then
        return
    fi

    # Avoid tracking shelltime commands themselves to prevent loops
    if [[ "$CMD" =~ ^shelltime ]]; then
        return
    fi

    # Ensure CMD is not empty or the precmd_invoke_cmd itself
    if [ -z "$CMD" ] || [ "$CMD" == "precmd_invoke_cmd" ]; then
        return
    fi

    shelltime track -s=bash -id=$SESSION_ID -cmd="$CMD" -p=post -r=$LAST_RESULT --ppid=$PPID &> /dev/null
}

# Set the functions for bash-preexec
preexec_functions+=(preexec_invoke_cmd)
precmd_functions+=(precmd_invoke_cmd)
