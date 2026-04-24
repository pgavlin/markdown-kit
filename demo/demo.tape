# VHS tape for the cmd/md hero GIF.
# Re-render with:  vhs demo/demo.tape
# Builds the md binary from the current working tree into a scratch dir so
# the gif always reflects HEAD, with no compilation output on the recorded
# terminal.

Output demo/md.gif

Set Shell bash
Set FontSize 14
Set Width 1200
Set Height 720
Set Padding 20
Set TypingSpeed 50ms
Set PlaybackSpeed 1.0
Set Theme "Dracula"

# Isolate md's config/cache/index from the host so the gif is reproducible.
Env XDG_CONFIG_HOME "/tmp/md-demo/config"
Env XDG_CACHE_HOME  "/tmp/md-demo/cache"
Env XDG_DATA_HOME   "/tmp/md-demo/data"

Hide
Type "export PS1='$ '"
Enter
Type "rm -rf /tmp/md-demo && mkdir -p /tmp/md-demo/config /tmp/md-demo/cache /tmp/md-demo/data /tmp/md-demo/bin"
Enter
Type "go build -o /tmp/md-demo/bin/md ./cmd/md >/dev/null 2>&1"
Enter
Type "export PATH=/tmp/md-demo/bin:$PATH"
Enter
Type "cd demo"
Enter
Type "clear && printf '\x1b[3J'"
Enter
Show

# --- Open the primary document ---
Type "md hello.md"
Enter
Sleep 1500ms

# --- Act 1: basic navigation ---
Type@120ms "jjjj"
Sleep 700ms
Type "}"
Sleep 500ms
Type "}"
Sleep 500ms
Type "{"
Sleep 700ms

# --- Act 2: in-doc search ---
Type "/"
Sleep 400ms
Type "code"
Enter
Sleep 1s
Type "n"
Sleep 500ms
Escape
Sleep 200ms
Escape
Sleep 400ms

# --- Act 3: TOC -> jump to Example -> item nav onto code block -> copy ---
Type "t"
Sleep 800ms
PageUp
Sleep 250ms
Down
Sleep 200ms
Down
Sleep 200ms
Down
Sleep 200ms
Enter
Sleep 900ms
Type "]"
Sleep 700ms
Type "y"
Sleep 1200ms

# --- Act 4: view source toggle ---
Ctrl+U
Sleep 1800ms
Ctrl+U
Sleep 800ms

# --- Act 5: open code.md in a new tab via the file picker ---
Ctrl+T
Sleep 900ms
Type "code"
Sleep 700ms
Enter
Sleep 1500ms

# Flip back to hello.md.
Shift+Tab
Sleep 1s

# --- Act 6: TOC -> jump to Reading section -> item nav onto link -> open link in new tab ---
Type "t"
Sleep 700ms
PageUp
Sleep 250ms
Down
Sleep 200ms
Down
Sleep 200ms
Enter
Sleep 800ms
Type "]"
Sleep 700ms
Type "T"
Sleep 2s

# --- Quit ---
Type "q"
Sleep 800ms
