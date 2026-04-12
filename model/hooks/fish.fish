#!/usr/bin/env fish

# Check if shelltime CLI exists
if not command -v shelltime > /dev/null
    echo "Warning: shelltime CLI not found. Please install it to enable time tracking."
else
    shelltime gc
end

# Create a timestamp for the session when the shell starts
set -g SESSION_ID (date +%Y%m%d%H%M%S)

# Capture parent process ID at shell startup (fish doesn't have native $PPID)
set -g FISH_PPID (ps -o ppid= -p %self | string trim)

# Define the preexec function
function fish_preexec --on-event fish_preexec
    if string match -q 'exit*' -- $argv; or string match -q 'logout*' -- $argv; or string match -q 'reboot*' -- $argv
        return
    end

    shelltime track -s=fish -id=$SESSION_ID -cmd="$argv" -p=pre --ppid=$FISH_PPID > /dev/null
end

# Define the postexec function
function fish_postexec --on-event fish_postexec
    set -g LAST_RESULT (echo $status)
    if string match -q 'exit*' -- $argv; or string match -q 'logout*' -- $argv; or string match -q 'reboot*' -- $argv
        return
    end
    # This event is triggered before each prompt, which is after each command
    shelltime track -s=fish -id=$SESSION_ID -cmd="$argv" -p=post -r=$LAST_RESULT --ppid=$FISH_PPID > /dev/null
end
