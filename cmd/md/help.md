# md — Interactive Markdown Reader

`md` is a terminal-based interactive Markdown reader. It renders Markdown
documents with full styling, syntax highlighting, and image support directly
in your terminal.

## Starting md

```
md [path or URL ...]
```

- **No arguments**: Opens a file picker in your current directory.
- **One or more files**: Opens each file in a separate tab.
- **URLs**: Fetches the URL, converts HTML to Markdown, and displays the result.
- **Mixed**: You can pass any combination of local files and URLs.

Supported file extensions: `.md`, `.markdown`, `.mdown`, `.mkdn`, `.mkd`,
`.mdwn`. Additional formats can be opened if format converters are configured
(see Configuration below).

## Scrolling and Movement

| Key | Action |
|-----|--------|
| {{.Down}} | Scroll down |
| {{.Up}} | Scroll up |
| {{.PageDown}} | Page down |
| {{.PageUp}} | Page up |
| {{.GotoTop}} | Go to top of document |
| {{.GotoEnd}} | Go to end of document |
| {{.Home}} | Go to top of document |
| {{.End}} | Reset column offset |
| {{.Left}} | Scroll left |
| {{.Right}} | Scroll right |

## Content Width

| Key | Action |
|-----|--------|
| {{.DecreaseWidth}} | Decrease content width by 10 |
| {{.IncreaseWidth}} | Increase content width by 10 |

The content width controls how wide the rendered Markdown body is within the
terminal. Decrease it for a narrower column; increase it for wider text.

## Element Navigation

Jump directly between headings, links, and code blocks:

| Key | Action |
|-----|--------|
| {{.NextLink}} | Jump to next link |
| {{.PrevLink}} | Jump to previous link |
| {{.NextHeading}} | Jump to next heading |
| {{.PrevHeading}} | Jump to previous heading |
| {{.NextCodeBlock}} | Jump to next code block |
| {{.PrevCodeBlock}} | Jump to previous code block |

## Link Following

`md` renders links as highlighted, navigable elements. Use {{.NextLink}} and
{{.PrevLink}} to move between links, then:

| Key | Action |
|-----|--------|
| {{.FollowLink}} | Follow the selected link |
| {{.GoBack}} | Go back to the previous page |
| {{.OpenBrowser}} | Open the selected link in your system browser |
| {{.NewTab}} | Open the selected link in a new tab |

When you follow a link:

- **Markdown files** (`.md`, etc.) are loaded and rendered directly.
- **URLs** are fetched; HTML content is converted to Markdown using the
  configured converter (builtin or external).
- **Convertible files** (if format converters are configured) are converted
  and rendered.
- **Anchor links** (`#heading-name`) scroll to the matching heading in the
  current document.

Each followed link is pushed onto a back stack, so you can press {{.GoBack}}
to return to the previous page. The back stack is per-tab.

## In-Document Search

| Key | Action |
|-----|--------|
| {{.Search}} | Open the search prompt |
| {{.NextMatch}} | Jump to the next match |
| {{.PrevMatch}} | Jump to the previous match |
| {{.ClearSearch}} | Clear search highlights and close search |

Type your query after pressing {{.Search}} and press `Enter` to search.
Matches are highlighted in the document. Use {{.NextMatch}} and
{{.PrevMatch}} to cycle through matches.

## Tabs

When multiple documents are open, a tab bar appears at the top of the screen.
Each tab shows the document's title (extracted from the first heading) or the
filename.

| Key | Action |
|-----|--------|
| {{.NextTab}} | Switch to the next tab |
| {{.PrevTab}} | Switch to the previous tab |
| {{.CloseTab}} | Close the current tab |
| {{.CloseAllTabs}} | Close all tabs (returns to file picker) |

## Opening Documents

| Key | Action |
|-----|--------|
| {{.OpenFile}} | Open the file picker |
| {{.OpenFileNewTab}} | Open the file picker (opens in a new tab) |
| {{.OpenURL}} | Open the URL input prompt |
| {{.NewTab}} | Open the selected link in a new tab |

### File Picker

The file picker provides fuzzy search over files in your current directory
(and subdirectories). Type to filter the list, then use `Up`/`Down` to
select a file and `Enter` to open it. Press `Esc` to dismiss the picker.

Only Markdown files and files matching configured format converter extensions
are shown.

Press `Tab` within the file picker to switch to URL input mode. Press `Tab`
again to switch back. This lets you toggle between opening a local file and
fetching a URL without dismissing the picker.

### URL Input

Press {{.OpenURL}} to open the URL input prompt. Type or paste a URL and
press `Enter` to fetch and display it. The URL is resolved and fetched via
HTTP; HTML responses are converted to Markdown.

You can also enter a URL from within the file picker by pressing `Tab` to
switch to URL mode.

## Page History

| Key | Action |
|-----|--------|
| {{.History}} | Open the history picker |

The history picker shows all pages visited in the current tab's back stack.
Select an entry to jump back to that page. This is useful when you've
followed many links and want to return to a specific earlier page without
pressing {{.GoBack}} repeatedly.

## Document Search

All documents you open in `md` are automatically indexed for full-text
search. This index persists across sessions, so you can find previously
opened documents later.

| Key | Action |
|-----|--------|
| {{.SearchDocuments}} | Open the document search picker |
| {{.FindSimilar}} | Find documents similar to the current document |

### Keyword Search

Press {{.SearchDocuments}} to open the document search picker. Type a query
to search across all previously opened documents using full-text keyword
search (powered by SQLite FTS5). Results show the document title, path, and
when it was last opened.

Use `Up`/`Down` to select a result and `Enter` to open it. Press `Esc` to
dismiss.

### Semantic Search

If an embedder is configured (see Configuration), you can press `Tab` in the
search picker to toggle between keyword and semantic search modes. Semantic
search finds documents by meaning rather than exact keywords, using vector
embeddings.

### Find Similar

Press {{.FindSimilar}} to find documents similar to the one you're currently
viewing. This uses vector embedding similarity to find related documents in
your index. Requires an embedder to be configured.

## View Source

| Key | Action |
|-----|--------|
| {{.ToggleRaw}} | Toggle raw Markdown source view |

Press {{.ToggleRaw}} to see the raw Markdown source of the current document,
displayed inside a fenced code block. Press {{.ToggleRaw}} again to return
to the rendered view.

## Copy

| Key | Action |
|-----|--------|
| {{.CopySelection}} | Copy the selected code block to clipboard |

When a code block is selected (navigate to one using {{.NextCodeBlock}} /
{{.PrevCodeBlock}}), press {{.CopySelection}} to copy its contents to your
system clipboard.

## Reload

| Key | Action |
|-----|--------|
| {{.Reload}} | Reload the current page |

Reloads the current document from disk or re-fetches the URL. Useful when a
file has been edited externally.

## Help

| Key | Action |
|-----|--------|
| {{.Help}} | Toggle the key binding overlay |
| {{.UserGuide}} | Open this user guide in a new tab |
| {{.BugReport}} | Copy a bug report to clipboard |

Press {{.Help}} to show a quick-reference overlay of all key bindings. Press
{{.Help}} again or `Esc` to dismiss it.

Press {{.UserGuide}} to open this full user guide as a new tab.

## Quitting

| Key | Action |
|-----|--------|
| {{.Quit}} | Quit |

## Configuration

`md` reads its configuration from a TOML file:

- **macOS**: `~/Library/Application Support/md/config.toml`
- **Linux**: `$XDG_CONFIG_HOME/md/config.toml` (default `~/.config/md/config.toml`)
- **Windows**: `%APPDATA%\md\config.toml`

A default configuration file is created automatically on first run. Use
`md config` to display the current configuration.

### Theme

```toml
theme = "dracula"
```

Set the color theme to any [Chroma style name](https://xyproto.github.io/splash/docs/).
Leave empty or omit for the built-in dark theme.

### HTML-to-Markdown Converter

```toml
[converter]
method = "builtin"
```

Controls how HTML content (from URLs) is converted to Markdown.

- `"builtin"` (default): Uses the built-in Go HTML-to-Markdown converter.
- `"external"`: Runs a shell command. The HTML is passed on stdin, and the
  command should write Markdown to stdout.

```toml
[converter]
method = "external"
command = "pandoc -f html -t markdown"
```

### Format Converters

Format converters allow `md` to open non-Markdown files by converting them
on the fly. Each converter specifies which file extensions and/or MIME types
it handles, and a shell command to perform the conversion.

```toml
[[converters]]
extensions = [".rst"]
mime_types = ["text/x-rst"]
command = "pandoc -f rst -t markdown $MD_INPUT -o $MD_OUTPUT"
```

The command receives the input file path in the `$MD_INPUT` environment
variable and should write Markdown to the path in `$MD_OUTPUT`. Both
variables are set automatically.

You can define multiple `[[converters]]` entries for different formats.

### Document Search Embedder

To enable semantic (vector) search and the "find similar" feature, configure
an embedder:

#### Ollama (local)

```toml
[search]
embedder = "ollama"

[search.ollama]
url = "http://localhost:11434"
model = "nomic-embed-text"
dimensions = 768
```

#### OpenAI-compatible API

```toml
[search]
embedder = "api"

[search.api]
url = "https://api.openai.com/v1/embeddings"
model = "text-embedding-3-small"
api_key_env = "OPENAI_API_KEY"
dimensions = 1536
```

The `api_key_env` field specifies the environment variable that holds your
API key (default: `OPENAI_API_KEY`).

#### External Command

```toml
[search]
embedder = "command"

[search.command]
command = "my-embed-tool"
dimensions = 384
```

The command receives document text on stdin and should write a JSON array of
floats to stdout.

### Custom Key Bindings

Override any key binding in the `[keys]` section. Each key accepts a string
or an array of strings:

```toml
[keys]
quit = ["q", "ctrl+c"]
help = "?"
up = ["k", "up"]
search_documents = "S"
```

Available binding names: `up`, `down`, `page_up`, `page_down`, `goto_top`,
`goto_end`, `home`, `end`, `left`, `right`, `next_link`, `prev_link`,
`next_code_block`, `prev_code_block`, `next_heading`, `prev_heading`,
`decrease_width`, `increase_width`, `follow_link`, `go_back`,
`copy_selection`, `search`, `next_match`, `prev_match`, `clear_search`,
`toggle_raw`, `open_url`, `open_browser`, `open_file_new_tab`, `next_tab`,
`prev_tab`, `close_tab`, `close_all_tabs`, `new_tab`, `reload`, `history`,
`search_documents`, `find_similar`, `user_guide`, `bug_report`, `help`, `quit`.

## Subcommands

### `md config`

Prints the resolved configuration as TOML to stdout.

### `md system clear-cache`

Removes all cached conversion results (HTML-to-Markdown conversions from
URLs and format converter outputs). The cache directory is in your system's
cache directory under `md/`.

### `md system clear-index`

Removes the document search index (the SQLite database that stores full-text
and vector search data). This clears all document history from the search
picker. Documents will be re-indexed as you open them.

## Data Storage

`md` stores data in platform-standard directories:

| Data | macOS | Linux |
|------|-------|-------|
| Config | `~/Library/Application Support/md/` | `~/.config/md/` |
| Cache | `~/Library/Caches/md/` | `~/.cache/md/` |
| Search index | `~/Library/Application Support/md/` | `~/.local/share/md/` |
| Log file | `~/Library/Application Support/md/` | `~/.local/share/md/` |

The log file (`md.log`) records diagnostic information for debugging. It is
overwritten on each run.
