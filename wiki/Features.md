# Features

## Messaging

- Real-time messages, edits, deletes, reactions, and typing indicators over WebSocket
- Apps' "working" status: "Atlas is working..." under the channel, and the app's own status text ("Atlas Thinking...") in the open thread it is working on
- Edit your own messages (`E`) — reuses the compose box with stash/restore for any in-progress draft
- Delete your own messages (`D`) — centered confirmation overlay with message preview
- Remove a link preview from your own message (`x`): the "x" on Slack's preview card. A confirmation names the preview (one button per preview when the message has several); the card disappears when Slack confirms the change. Works from the messages pane and from a thread reply. Slack decides what it allows, so removing the card of a message you forwarded can be refused.
- Live message updates: an edit, a link preview Slack adds a moment after you post, or a preview you remove all update the message in place. Only a real edit adds the "(edited)" mark.
- Slack markdown rendering (bold, italic, strikethrough, code, blockquotes, links, mentions)
- User mentions in channel and thread content, including forwarded/shared previews, resolve to display names from the workspace cache or in the background and refresh in place. Unresolved users retain their Slack ID; literal `@ID` text is left unchanged.
- Channel mentions in channel and thread content, including forwarded/shared previews, resolve from workspace metadata or in the background—even for accessible public channels you haven't joined—and refresh in place. Resolving a mention does not join the channel or add it to the sidebar. Channels whose names cannot be resolved still show `#unknown`. Failed lookups cool down for 30 seconds before a later request can retry; known names and cache hits bypass the cooldown. Cooldown state stays in memory, with expired records pruned on subsequent lookup failures.
- Emoji shortcodes (`:rocket:` → 🚀), including Block Kit headers/text and button/accessory labels (`:white_check_mark: Resolve` → `✅ Resolve`); plain headers and control labels are not interpreted as markdown. Long headers clip to display width without breaking emoji image placements.
- In image-emoji mode, workspace custom definitions take precedence over built-in names, including when following emoji aliases. Author-status emoji in channel and thread message headers use the same image renderer as the message body. If a status image fails to load, its existing glyph fallback remains visible.
- Day separators (Today, Yesterday, Monday, full date)
- Infinite scroll backfill into SQLite cache
- Search: vim-style in-channel search (`/`, `n`/`N`) over cached history, plus server-side workspace search (`Ctrl+f`) with `from:` / `in:` / `before:` modifiers
- New-message landmark (red `── new ──` line at the unread boundary)
- Mark-as-read synced to Slack on channel entry
- Mark-as-unread (`U`) — rolls the read watermark backward to the selected message; thread replies supported. Inbound `channel_marked` / `thread_marked` events from other Slack clients are reflected live.
- Edited / threaded message indicators
- ANSI-aware wrapping and truncation (no broken color codes mid-line)
- Drag-to-copy: drag the mouse across messages to highlight them; release to copy plain text to the system clipboard via OSC 52
- Copy message text (`y`) and copy permalink (`Y` / `C`) to the system clipboard via OSC 52. On an Enterprise Grid workspace that refuses Slack's permalink API, slk builds the link itself, as Slack's own client does, so copying still works
- Copy huddle link (`H` or `:huddle`) — the `https://app.slack.com/huddle/<TEAM>/<CHANNEL>` deep link Slack's own "Copy huddle link" produces, for the active conversation (channels, DMs and group DMs; a thread copies its parent's). Opening it joins the live huddle or shows Slack's huddle panel for that conversation. The profile dialog (`K`) also offers `c` / a click on the 📋 on the "🎧 In a huddle" row when Slack reported which channel the person's huddle is in.
- Forward a message or thread reply (`F`) — the same fuzzy picker as `Ctrl+t`, limited to joined channels and existing DMs in the current workspace. Enter shares the original permalink with Slack preview unfurling enabled; Esc cancels. Keeps your current channel and draft unchanged. Slack controls preview availability and access to the original message. Pressing Enter on the forwarded message navigates to the original.

## Compose

![Composing a DM to Sam and getting a live reply](https://github.com/gammons/slk/raw/main/docs/assets/demo/compose.gif)

- Multi-line input, `Shift+Enter` for newlines
- External editor (`Ctrl+E`) — opens the draft in `$VISUAL`, then `$EDITOR`, then `[compose] editor` from config; the edited text replaces the draft when the editor exits
- Inline `@mention` autocomplete (resolves to `<@UserID>` on send)
- Slash commands, with Slack's `/` menu: type `/` in an empty composer for the workspace's commands, each with its usage, its app and what it does; type to narrow, `Tab` or `Enter` to pick. See [[Commands]]. A command runs in the workspace Slack lists it under, which on Enterprise Grid is not the organization the session signs in to
- Special mentions: `@here`, `@channel`, `@everyone`
- Bracketed paste — paste multi-line text from the system clipboard without it being interpreted as keystrokes
- Smart paste (`Ctrl+V`) — pastes a clipboard image as an attachment, or a copied file path as an attached file, or falls through to text. Multiple attachments + caption send together via Slack's V2 file-upload API. Note: use `Ctrl+V` (not your terminal's `Ctrl+Shift+V` paste shortcut) — terminal-initiated paste only delivers text, never image bytes.
- CommonMark in compose: type `**bold**`, `~~strike~~`, `[label](url)`, `- list items`, `1. numbered`, or fenced ```code blocks``` and slk converts them on send to Slack's mrkdwn + rich_text format. Already-mrkdwn syntax (`*bold*`, `_italic_`, `~strike~`) passes through unchanged and receives the corresponding rich-text styling.
- Unsent drafts stay with their conversation. Text and attachments are kept separately for each workspace, channel, DM and thread, and come back when you switch to it again. Drafts are in-session only — nothing is written to disk and nothing is synced to Slack — and a draft's attachments stay in memory until you send or remove them. While a file upload is in progress, navigation that would switch the composer is refused, so the upload's caption and attachments stay with the conversation they were sent from.

## Images

- Inline image attachments render automatically in the messages pane: kitty graphics protocol on capable terminals (kitty, ghostty, recent WezTerm), sixel on foot/mlterm and on any terminal that advertises sixel in its DA1 reply (xterm with sixel support, DomTerm, toyterm, …), half-block (`▀`) fallback everywhere else. Several images in one message sit side by side and wrap onto the next row when the next image does not fit; a single image keeps its usual size, and each image still opens on its own
- User avatars use the same image path as inline images for sharper pixels: kitty graphics on capable terminals, sixel on sixel terminals, and half-block elsewhere (or for a sixel avatar the pane edge cuts). Long author names wrap in full beside the avatar when the pane narrows; the avatar's two rows stay contiguous, and the message body starts below the complete header. Timestamps stay whole when they fit on a header line, so selection can add the date when space allows without changing the row count.
- Opt-in Slack layout for the message pane (`[appearance] slack_style`): an author's messages sent within five minutes share one header, and a conversation shorter than the pane sits at its bottom, by the input. An edited message, one with no text, and the first unread one keep their own header
- A DM, group DM or app is named without the channel's `#`: in the pane header, the status bar and the input's placeholder
- Click any inline image (or press `Space` or `O` on the selected message) for a full-screen in-app preview; `Space` closes it again
- `Enter` from the preview launches the OS image viewer
- Lazy-loaded: images download only as they scroll into view
- LRU cache at `~/.cache/slk/images/` (default 200 MB cap)
- Inside tmux, slk falls back to half-block to avoid pixel-protocol pass-through pitfalls
- Configurable via `[appearance] image_protocol` (`auto` / `kitty` / `sixel` / `halfblock` / `off`) and `max_image_rows`

See [[Terminal Compatibility|Terminal-Compatibility]] for which protocol your terminal supports.

## Threads

![Opening a deploy thread, a live reply arriving, and replying in the thread](https://github.com/gammons/slk/raw/main/docs/assets/demo/threads.gif)

- Opens beside the channel (at least 80 columns wide) with `Enter`, toggled
  with `Ctrl+]`. On terminals too narrow to fit both panes, the thread and
  channel stack instead: only one is shown at a time, and focus decides which
  — `Tab` / `Shift+Tab` switch between them, `Esc` or `q` closes the thread.
  The header is a breadcrumb naming the channel, the thread's author, and the
  reply count (e.g. `# general › Thread from alice · 2 replies`).
- Full screen: `f` gives the focused pane (the thread, the channel or the
  channel list) the whole terminal; `f` again returns to the panes.
- Live thread reply routing, real-time updates
- Emoji images in the thread parent render even when there are no replies; they do not depend on another message having uploaded the image first.
- Also send to channel: `Ctrl+O` while composing a thread reply toggles Slack's
  "Also send to #channel" broadcast (`reply_broadcast=true`), and `Alt+Enter`
  sends with broadcast in a single keystroke. An accent-colored
  `↪ also send to #channel` line under the input shows when it's armed; the
  toggle is non-sticky (cleared after send or when opening another thread).
- Closes on channel switch
- **Threads view** (`⚑ Threads` at top of sidebar): scrollable list of every
  thread you authored, replied to, or were @-mentioned in for the active
  workspace. Unread first, then newest activity. Each thread is drawn as
  Slack draws it: the channel and the people in the thread, then a card
  with the parent message, a `Show N more replies` link and the two
  newest replies (a thread whose replies slk has not fetched yet shows
  its reply count and when the last one came). Selecting a thread opens
  it in the side panel; the list re-ranks live as new replies arrive.
  v1 is computed from the local SQLite cache, so threads from channels
  you have not yet opened in slk will not appear until they are seen.

## Activity

- **Activity view** (`◉ Activity` in the sidebar, below Threads): the same
  feed as the Activity tab in the official client — @mentions (including
  `@here`/`@channel`, user groups and keywords), replies in threads you
  follow, reactions to your messages, and DMs. Open it from the sidebar,
  with `Ctrl+a` (which toggles back to your channel), or with `:activity`.
- Each item is a card laid out as in Slack, beside the author's avatar:
  who and how long ago (`3 mins`), bold with a dot while unread; the
  message itself — rendered like the message pane, with emoji, mentions and
  channel links; then where it happened (`Direct message`,
  `Mention in #general`, `Thread in ◆ private`). A reaction reads
  `Reacted 🎉 to: …`; DMs are headed by the other person, and your own
  latest message reads `You: …`. A rule separates the cards, and the
  heading counts the unread ones
- `Enter` opens the item: a thread reply opens the thread in the side panel,
  anything else jumps to the message in its channel
- `u` toggles unread-only (filtered by Slack, not locally)
- Shows the 50 most recent items. The sidebar badge counts the unread ones;
  it refreshes when the workspace connects, when you switch workspaces, and
  whenever you open the view

## Reactions

![Teammates' reactions arriving live, then adding one with the reaction picker](https://github.com/gammons/slk/raw/main/docs/assets/demo/reactions.gif)

- Search-first picker overlay (`r`) with frecent emoji
- Quick-toggle nav across existing pills (`R`, then `h/l/Enter`)
- Pill-style display (green = yours, gray = others)
- Optimistic UI, deduped against the WebSocket echo

## Channels & Workspaces

![A workspace's unread badge lighting up in the rail, then switching to it](https://github.com/gammons/slk/raw/main/docs/assets/demo/workspaces.gif)

- Three-panel layout: workspace rail, channel sidebar, message pane
- Public (`#`), private (`◆`), DM (`●`/`○` for presence, `⊘` while the other person is in Do Not Disturb), and group DM channels
- **Slack-native sidebar sections** — slk reads your sections directly from Slack and reflects them live: section names, emoji, linked-list order, and channel/DM membership are kept in sync via the same WebSocket events the official client uses. Reorder, rename, create, or delete sections in any other Slack client; slk catches up within a couple seconds. Read-only: section editing still happens in the official client. Falls back to glob-based config sections when disabled or if the API is unavailable.
- Collapsible sections — `Enter`/`Space` on a section header toggles it. A collapsed header shows aggregate unreads (`▸ Channels •3`). With Slack's sections every section starts expanded; with `[sections.*]` from config the Channels and Apps sections start collapsed. `[sidebar] start_collapsed = true` starts every section collapsed. Which sections are collapsed is saved per workspace (`collapsed_sections` in `config.toml`) and restored on the next start. Opening a channel in a collapsed section shows that channel under the header without expanding the section. With `[sidebar] show_unread_in_collapsed = true` a collapsed section also lists its unread channels, and its header badge still counts them
- App DMs sit in Slack's Apps section, where Slack's own sidebar puts them
- With `[appearance] full_names`, people are shown by full name wherever slk names them (sidebar, message authors, mentions, the input's placeholder), as Slack does by default. The cache keeps both names, so the option switches at the next start with nothing to refetch; a person cached before it existed is fetched once more when next seen
- A group DM lists every other member by name, alphabetically as Slack does; members slk has not met are fetched in one batch at start
- Close a DM, group DM or app's DM as Slack's "x" does: `x` on its sidebar row, or, with `[sidebar] slack_style`, a click on the "✕" that shows at the row's right end while the pointer is over it (a row with an unread badge shows the badge instead). The close goes to Slack, so the row leaves Slack's sidebar too, and a conversation closed or opened again in Slack leaves or rejoins slk's while it runs
- Opt-in Slack look (`[sidebar] slack_style`, `channel_indent`, `avatars`): headers start at their emoji, rows sit indented under them, each DM and app shows its avatar with the peer's presence on its corner (avatars need kitty graphics), and the row of what is open is filled. With `avatars`, a group DM shows the number of people in it on a square the size of an avatar, and mention badges are pills with a gap between the badges of adjacent rows
- With `slack_style`, the Direct messages and Apps sections list unread conversations first, then the one with the newest message, with your DM with yourself last, as Slack does, and re-sort as messages arrive or are read. Other sections stay alphabetical
- One row is marked as open: the Threads or Activity row while that view is up, else the open conversation's
- With `slack_style`, Channels, Direct messages and Apps carry Slack's icon where a custom section has its emoji (with `avatars`; otherwise the column is left blank), so every section's name starts in one column. Your DM with yourself is marked "you"
- With `avatars`, a channel's `#` and a private channel's lock are drawn as images in the avatar's cells, so the icon column lines up whatever the rows are: avatars, group squares, channels
- Avatars, emoji and pictures stay up behind an open dialog (the reaction picker, the finder): only text and backgrounds are dimmed
- Images keep their proportions in their cells. A cell is rarely exactly twice as tall as wide, and an image stretched over its cells came out taller than wide (9% on 17x37 px cells), which showed on every emoji and avatar
- With `avatars`, a DM whose avatar is not loaded shows a plain figure with the peer's presence, as Slack does for someone with no photo
- Two slk in one terminal (two tmux windows) keep their own images: kitty image IDs start at a random point per run
- Inside tmux, images uploaded while another tmux window was up are sent again when slk's window comes back: tmux forwards an upload only from a visible pane (`allow-passthrough on`), which otherwise leaves the image's cells blank. `allow-passthrough all` avoids the loss altogether
- A link preview's picture or a Block Kit image (a `/giphy` post) replaces its "loading…" placeholder as soon as it downloads, and is fetched with a browser User-Agent, which some image hosts require
- Live unread indicators: bold + blue dot for unread channels, muted text for read ones, aggregate dot+count on collapsed section headers
- Glob-based config sections (`[sections.*]` in `config.toml`) — used when `use_slack_sections = false` or as a fallback when Slack's API is unreachable. Channel patterns can carry an optional `":<N>"` suffix (e.g. `"eng-general:1"`) to pin order within a section; see [Configuration › Ordering channels within a section](Configuration.md#ordering-channels-within-a-section).
- Deactivated people are hidden like in the Slack desktop client: their DMs leave the sidebar and they are not offered in the new-message picker or `@` / `#` autocomplete, while old messages still show them as authors. The channel finder shows their DM only when you type their name, marked `deactivated`, so you can still read the history; its composer is disabled. Turn it off with `[general] hide_deactivated_users = false`; see [Configuration › Deactivated users](Configuration.md#deactivated-users).
- Fuzzy channel finder (`Ctrl+t` / `Ctrl+p`) — opens on what is unread, under "Unreads" with each count, then on where you were last, under "Recent", as Slack's switcher does; shows the channel you open under its collapsed section's header; ranks 1:1 DMs above group DMs when searching by person name
- Workspace picker (`:ws`) and direct jump (`1`–`9`)
- `:` command line with Tab completion (`Shift+Tab` goes back) and the matching names listed in the status bar; `:help` (same as `?`) lists every key and command, `:version` shows the running build
- All workspaces stay connected in parallel for live unread badges

## Marks & Navigation

- **Vim-style marks** — `m` + a letter records the selected message; `'` + that letter jumps back to it. A mark captures the channel, the message, and, when set inside a thread, the thread and the reply, so a mark can return you to one message in a 200-reply thread. In the Threads view, a mark records the selected thread.
- Session vs. persistent, as in vim: lowercase marks (`ma`–`mz`) die with the session, uppercase (`mA`–`mZ`) survive a restart. `persist_all` makes lowercase persist too, non-destructively.
- `''` back-jump — returns you to where you were before the last jump, whether or not it changed channel; press again to toggle back.
- `:marks` overlay — every mark with its channel and a preview of the message, rendered from a snapshot taken at mark time, so it works offline and immediately after a restart. Click a row (or press `Enter`) to jump to it, `Backspace` deletes a row, and `:delmarks abc` deletes by letter.
- **Position-aware back/forward** (`Ctrl+h` / `Ctrl+l`) — retraces visited channels and restores the message you were reading when you left each one, rather than dumping you at the newest message.

## Notifications

- OS-level desktop notifications via [beeep](https://github.com/gen2brain/beeep)
- Triggers on DMs, mentions, and configurable keywords
- Suppressed when you're focused on the relevant channel
- Suppressed entirely while you're in DND/snooze
- Never for an author you hid with `:suppress` (per conversation or workspace-wide); their messages are not shown either

## Status & DND

- Set self presence (Active / Away) and DND/snooze from `Ctrl+S`
- Standard snooze durations (20m / 1h / 2h / 4h / 8h / 24h / until tomorrow morning) plus custom minutes
- Workspace status presets (Working from home / On vacation) and Clear status, independent of presence — picking Active/Away never clears an existing status. Per-workspace, like presence and DND.
- Live status segment in the status bar with snooze countdown
- Reflects external state changes — set from the official Slack client or via your own API scripts — in real time over the WebSocket
- Shows other people's state too: their custom status emoji (🎧 instead while they are in a huddle) follows their name on DM rows, message authors and channel-finder rows, `⊘` replaces the presence dot while they are in DND, and an open DM's header shows the full status text and when their DND ends
- Statuses and DND disappear when they expire; a huddle is re-checked every minute, because Slack does not always announce that one ended

## Export

- Save the open thread to Markdown with `S` (see [[Keybindings]])
- Export one channel's messages and threads for a date range, without
  launching the TUI:

  ```sh
  slk export \
    --workspace example-workspace \
    --channel project_alpha \
    --since 2026-04-01 \
    --until 2026-07-01 \
    --timezone America/New_York \
    --output ./project-alpha-export \
    --overlap 14
  ```

  Each conversation (a standalone message, or a thread with its replies)
  becomes one Markdown file, and `index.md` links them by day.
  - `--since` is inclusive and `--until` exclusive, both read as midnight in
    `--timezone` (default: local). `--until` defaults to tomorrow.
  - `--overlap N` widens the range by N calendar days on each side.
  - A reply inside the range is exported even when its thread started
    earlier; the older parent is kept, labelled, as context. Finding those
    threads means scanning the channel's whole earlier history, so a first
    export of a long-lived channel takes a while.
  - `--workspace` takes a slug, team ID or name, and may be omitted with a
    single workspace. `--channel` takes a name or ID. `--output` must be
    empty or absent, and defaults to a folder under the exports directory.

## Connectivity

- Browser-cookie auth (`xoxc` + `d`) — works as any user, no Slack App required
- Direct connection to Slack's internal browser WebSocket protocol
- Auto-reconnect with exponential backoff (1s → 30s)
- Three-state connection indicator in the status bar

## Customization

![Switching between themes with the theme switcher](https://github.com/gammons/slk/raw/main/docs/assets/demo/themes.gif)

- 59 built-in themes (including `ANSI Dark` / `ANSI Light` that inherit your terminal palette)
- Drop-in custom themes (`~/.config/slk/themes/*.toml`)
- Live theme switcher (`Ctrl+y`); `Alt+y` / `Alt+Shift+y` cycle to the next / previous theme
- TOML config for appearance, animations, notifications, and channel sections
- Opt-in IRC-like "compact" style with less padding, no avatars, and same-line usernames via `appearance.compact`
- Deterministic per-user username coloring, opt-in via `colored_usernames`

See [[Configuration]] for the full `config.toml` reference and [[Keybindings]] for the key map.
