# VHS tape for the cmd/md hero GIF.
# Re-render with:  vhs demo/demo.tape
# Uses the in-tree md binary via `go run ../cmd/md` so the gif always
# reflects the current working tree.

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
Type "rm -rf /tmp/md-demo && mkdir -p /tmp/md-demo/config /tmp/md-demo/cache /tmp/md-demo/data"
Enter
Type "cd demo"
Enter
Type "clear"
Enter
Show

# --- Open the primary document ---
Type "go run ../cmd/md hello.md"
Enter
Sleep 4s

# --- Act 1: basic navigation ---
Type@120ms "jjjj"
Sleep 800ms
Type "}"
Sleep 600ms
Type "}"
Sleep 600ms
Type "{"
Sleep 800ms

# --- Act 2: table of contents ---
Type "t"
Sleep 1s
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms
Enter
Sleep 1500ms

# --- Act 3: item nav + in-doc search ---
Type "]"
Sleep 400ms
Type "]"
Sleep 600ms
Type "/"
Sleep 400ms
Type "code"
Enter
Sleep 1s
Type "n"
Sleep 600ms
Escape
Sleep 500ms

# --- Act 4: copy a code block ---
# Land item selection on the Go code block, then copy with `y`.
Type "]"
Sleep 400ms
Type "]"
Sleep 400ms
Type "]"
Sleep 600ms
Type "y"
Sleep 1500ms

# --- Act 5: view source toggle ---
Ctrl+U
Sleep 2s
Ctrl+U
Sleep 1s

# --- Act 6: multi-doc workflow ---
# Open code.md in a new tab via the file picker.
Ctrl+T
Sleep 1s
Type "code"
Sleep 800ms
Enter
Sleep 2s

# Flip back to hello.md.
Shift+Tab
Sleep 1200ms

# Navigate to the relative link to code.md, then open it in a new tab.
Type "["
Sleep 300ms
Type "["
Sleep 400ms
Type "T"
Sleep 2s

# --- Quit ---
Type "q"
Sleep 1s
