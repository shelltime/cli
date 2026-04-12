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

# Define the preexec function
preexec() {
    local CMD=$1
    # Check if command starts with exit, logout, or reboot
    if [[ $CMD =~ ^(exit|logout|reboot) ]]; then
        return
    fi

    shelltime track -s=zsh -id=$SESSION_ID -cmd="$CMD" -p=pre --ppid=$PPID &> /dev/null
}

# Define the postexec function (in zsh, it's called precmd)
precmd() {
    local LAST_RESULT=$?
    local CMD=$(fc -ln -1)
    # Check if command starts with exit, logout, or reboot
    if [[ $CMD =~ ^(exit|logout|reboot) ]]; then
        return
    fi
    shelltime track -s=zsh -id=$SESSION_ID -cmd="$CMD" -p=post -r=$LAST_RESULT --ppid=$PPID &> /dev/null
}
