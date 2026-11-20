package slackclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/core"
)

// capturedForm loads a scrubbed web-client capture and returns its form
// fields minus token and the _x_* envelope, which postForm and slackhttp
// own.
func capturedForm(t *testing.T, name string) url.Values {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Form [][2]string `json:"form"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	out := url.Values{}
	for _, kv := range c.Form {
		if kv[0] == "token" || strings.HasPrefix(kv[0], "_x_") {
			continue
		}
		out.Set(kv[0], kv[1])
	}
	return out
}

var clientTokenRe = regexp.MustCompile(`^web-[0-9]{13}$`)

// assertFormMatchesCapture compares field sets: same names, same values
// (JSON byte for byte), nothing extra. client_token is checked for shape
// only: it is a timestamp.
func assertFormMatchesCapture(t *testing.T, got url.Values, captureFile string) {
	t.Helper()
	want := capturedForm(t, captureFile)
	if tok := got.Get("client_token"); !clientTokenRe.MatchString(tok) {
		t.Errorf("client_token = %q, want web-<13 digits>", tok)
	}
	for k := range want {
		if k == "client_token" {
			continue
		}
		if got.Get(k) != want.Get(k) {
			t.Errorf("%s =\n  %q\nwant\n  %q", k, got.Get(k), want.Get(k))
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("unexpected field %s=%q (not in %s)", k, got.Get(k), captureFile)
		}
	}
}

func TestCommandForm_MatchesCaptures(t *testing.T) {
	cases := []struct {
		capture string
		req     CommandRequest
	}{
		{"chat_command_channel.json", CommandRequest{ChannelID: "C0EXAMPLE01", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"}},
		{"chat_command_thread.json", CommandRequest{ChannelID: "C0EXAMPLE02", ThreadTS: "1791758997.192499", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"}},
		{"chat_command_thread_no_args.json", CommandRequest{ChannelID: "C0EXAMPLE02", ThreadTS: "1791758997.192499", Command: "/giphy", TeamID: "T0EXAMPLE01"}},
		{"chat_command_unknown.json", CommandRequest{ChannelID: "C0EXAMPLE02", ThreadTS: "1791758997.192499", Command: "/notarealcommand"}},
		{"chat_command_shrug.json", CommandRequest{ChannelID: "D0EXAMPLE01", Command: "/shrug", Args: "hi", TeamID: "T0EXAMPLE01"}},
	}
	for _, tc := range cases {
		t.Run(tc.capture, func(t *testing.T) {
			got, err := commandForm(tc.req, newClientToken())
			if err != nil {
				t.Fatal(err)
			}
			assertFormMatchesCapture(t, got, tc.capture)
		})
	}
}

// A name followed only by whitespace has no arguments, so blocks is
// omitted exactly as for bare /giphy.
func TestCommandForm_WhitespaceOnlyArgsOmitBlocks(t *testing.T) {
	got, err := commandForm(CommandRequest{ChannelID: "C1", Command: "/giphy", Args: "   \n "}, "web-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["blocks"]; ok {
		t.Errorf("blocks = %q, want no blocks field", got.Get("blocks"))
	}
}

// Mentions arrive in wire form and become typed rich-text elements, and
// text is not HTML-escaped (the web client sends it verbatim).
func TestCommandForm_ArgsMentionsAndNoHTMLEscape(t *testing.T) {
	got, err := commandForm(CommandRequest{ChannelID: "C1", Command: "/invite", Args: "<@U0EXAMPLE01> a<b"}, "web-1")
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[{"type":"user","user_id":"U0EXAMPLE01"},{"type":"text","text":" a<b"}]}]}]`
	if got.Get("blocks") != want {
		t.Errorf("blocks =\n  %s\nwant\n  %s", got.Get("blocks"), want)
	}
}

func TestRunCommand_PostsToChatCommand(t *testing.T) {
	var gotPath string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"keep_input":false}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C0EXAMPLE01", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"})
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if gotPath != "/api/chat.command" {
		t.Errorf("path = %q, want /api/chat.command", gotPath)
	}
	if gotForm.Get("command") != "/giphy" || gotForm.Get("token") != "xoxc-test" {
		t.Errorf("form = %v, want command=/giphy and the xoxc token", gotForm)
	}
}

// TestRunCommand_SendsTheCapturedReason pins _x_reason to the value the web
// client sent on chat.command, read straight from the capture (no filter).
func TestRunCommand_SendsTheCapturedReason(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "chat_command_channel.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Form [][2]string `json:"form"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, kv := range capture.Form {
		if kv[0] == "_x_reason" {
			want = kv[1]
		}
	}
	if want == "" {
		t.Fatal("chat_command_channel.json has no _x_reason")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if got := r.FormValue("_x_reason"); got != want {
			t.Errorf("_x_reason = %q; want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"keep_input":false}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)
	c.teamID = "T0EXAMPLE01"
	c.Envelope().SetTeamID("T0EXAMPLE01")

	if err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C0EXAMPLE01", Command: "/giphy", Args: "gorilla", TeamID: "T0EXAMPLE01"}); err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
}

func TestRunCommand_RejectionIsTyped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"error":"unknown_command"}`))
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C1", Command: "/notarealcommand"})
	var rej *CommandRejectedError
	if !errors.As(err, &rej) || rej.Code != "unknown_command" {
		t.Fatalf("err = %v, want *CommandRejectedError{Code: unknown_command}", err)
	}
}

func TestRunCommand_HTTPFailureIsNotARejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	err := c.RunCommand(context.Background(), CommandRequest{ChannelID: "C1", Command: "/giphy"})
	var rej *CommandRejectedError
	if err == nil || errors.As(err, &rej) {
		t.Fatalf("err = %v, want a non-rejection transport error", err)
	}
}

// TestAppCommands_ReadsTheCapturedList parses the list the web client
// got for its "/" menu, and asks for it the way that client did.
func TestAppCommands_ReadsTheCapturedList(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "client_app_commands.json"))
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Form     [][2]string     `json:"form"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	wantReason := ""
	for _, kv := range capture.Form {
		if kv[0] == "_x_reason" {
			wantReason = kv[1]
		}
	}
	if wantReason == "" {
		t.Fatal("client_app_commands.json has no _x_reason")
	}

	var gotPath, gotReason, gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotReason, gotToken = r.FormValue("_x_reason"), r.PostForm.Get("token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(capture.Response)
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	list, err := c.AppCommands(context.Background())
	if err != nil {
		t.Fatalf("AppCommands: %v", err)
	}
	if gotPath != "/api/client.appCommands" || gotToken != "xoxc-test" {
		t.Errorf("request went to %q with token %q; want /api/client.appCommands and the xoxc token", gotPath, gotToken)
	}
	if gotReason != wantReason {
		t.Errorf("_x_reason = %q; want the captured %q", gotReason, wantReason)
	}

	byName := map[string]core.SlashCommand{}
	aliases, kinds := 0, map[string]int{}
	for _, cmd := range list {
		byName[cmd.Name] = cmd
		kinds[cmd.Type]++
		if cmd.AliasOf != "" {
			aliases++
		}
	}
	if len(list) != 32 || aliases != 5 || kinds[core.SlashCommandCore] != 24 || kinds[core.SlashCommandApp] != 7 || kinds[core.SlashCommandService] != 1 {
		t.Errorf("parsed %d commands, %d aliases, kinds %v; the capture has 32, 5, and 24 core, 7 app, 1 service", len(list), aliases, kinds)
	}
	if got, want := byName["/shrug"], (core.SlashCommand{Name: "/shrug", Usage: "your text", Desc: "Appends ¯\\_(ツ)_/¯ to your message", Type: "core", TeamID: "T0EXAMPLE01"}); got != want {
		t.Errorf("/shrug = %+v; want %+v", got, want)
	}
	if got := byName["/giphy"]; got.Type != core.SlashCommandService || got.AppName != "Giphy" {
		t.Errorf("/giphy = %+v; want the service command named for Giphy", got)
	}
	if got := byName["/linear"]; got.Type != core.SlashCommandApp || got.AppName != "Linear" || got.Usage != "issue title (optional)" {
		t.Errorf("/linear = %+v; want the Linear app's command with its usage trimmed", got)
	}
	if got := byName["/dm"]; got.AliasOf != "/msg" {
		t.Errorf("/dm = %+v; want an alias of /msg", got)
	}
}

func TestAppCommands_ReportsARefusal(t *testing.T) {
	if _, err := parseAppCommands([]byte(`{"ok":false,"error":"enterprise_is_restricted"}`)); err == nil || !strings.Contains(err.Error(), "enterprise_is_restricted") {
		t.Errorf("err = %v; want Slack's error", err)
	}
	if _, err := parseAppCommands([]byte(`<html>`)); err == nil {
		t.Error("a body that is not JSON parsed")
	}
	list, err := parseAppCommands([]byte(`{"ok":true,"commands":[{"name":"/a","teams":[]},{"name":"shortcut","type":"app"}]}`))
	if err != nil || len(list) != 1 || list[0].Name != "/a" || list[0].TeamID != "" {
		t.Errorf("list = %+v, err = %v; want /a alone, with no team", list, err)
	}
}

// TestEmojiUsage_ReadsCountsAndTheFrecencyStore pins the request (the
// web client's fetch-frecency-prefs, plus the store's name on
// Enterprise Grid) and the two string-wrapped JSON prefs it reads.
func TestEmojiUsage_ReadsCountsAndTheFrecencyStore(t *testing.T) {
	store := `{"":[{"id":"Etada","count":9,"visits":[5]}],` +
		`"pepe":[{"id":"Epepe_ez","count":12,"visits":[1,2]}],` +
		`"Epepe_ez":{"id":"Epepe_ez","count":46,"visits":[1700000000000,1794502042124,1790000000000]},` +
		`"E1f44c":{"id":"E1f44c","count":701,"visits":[1794255675716]},` +
		`"C0EXAMPLE01":{"id":"C0EXAMPLE01","count":3,"visits":[1]},` +
		`"Eodd":{"id":"Esomething-else","count":3,"visits":[1]}}`
	body, err := json.Marshal(map[string]any{"ok": true, "prefs": map[string]string{
		"emoji_use":           `{"tada":210,"pepe-salute":102,"+1::skin-tone-3":4}`,
		"frecency":            "",
		"frecency_ent_jumper": store,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var gotPath, gotPrefs, gotReason string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = r.ParseForm()
		gotPrefs, gotReason = r.PostForm.Get("prefs"), r.FormValue("_x_reason")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	c := NewClient("xoxc-test", "d-cookie")
	pointClientAtTestServer(t, c, srv)

	usage, err := c.EmojiUsage(context.Background())
	if err != nil {
		t.Fatalf("EmojiUsage: %v", err)
	}
	if gotPath != "/api/users.prefs.get" || gotPrefs != "emoji_use,frecency,frecency_ent_jumper" || gotReason != "fetch-frecency-prefs" {
		t.Errorf("request = %s prefs=%q reason=%q; want users.prefs.get for the three prefs as fetch-frecency-prefs", gotPath, gotPrefs, gotReason)
	}
	if len(usage.Counts) != 3 || usage.Counts["tada"] != 210 || usage.Counts["pepe-salute"] != 102 || usage.Counts["+1::skin-tone-3"] != 4 {
		t.Errorf("Counts = %v; want the three counts", usage.Counts)
	}
	want := []EmojiFrecency{
		{ID: "1f44c", Count: 701, LastUsed: 1794255675716},
		{ID: "pepe_ez", Count: 46, LastUsed: 1794502042124},
	}
	if len(usage.Recent) != len(want) || usage.Recent[0] != want[0] || usage.Recent[1] != want[1] {
		t.Errorf("Recent = %+v; want %+v (the emoji records only, each with its latest visit)", usage.Recent, want)
	}
}

func TestParseEmojiUsage(t *testing.T) {
	for name, body := range map[string]string{
		"no prefs":       `{"ok":true,"prefs":{}}`,
		"empty prefs":    `{"ok":true,"prefs":{"emoji_use":"","frecency":""}}`,
		"a broken store": `{"ok":true,"prefs":{"emoji_use":"","frecency_ent_jumper":"not json"}}`,
	} {
		usage, err := parseEmojiUsage([]byte(body))
		if err != nil || usage.Counts == nil || len(usage.Counts) != 0 || len(usage.Recent) != 0 {
			t.Errorf("%s: usage = %+v, err = %v; want empty results", name, usage, err)
		}
	}
	// Outside Enterprise Grid the store is the frecency pref.
	usage, err := parseEmojiUsage([]byte(`{"ok":true,"prefs":{"emoji_use":"{}","frecency":"{\"Etada\":{\"id\":\"Etada\",\"count\":2,\"visits\":[7]}}"}}`))
	if err != nil || len(usage.Recent) != 1 || usage.Recent[0] != (EmojiFrecency{ID: "tada", Count: 2, LastUsed: 7}) {
		t.Errorf("frecency pref: usage = %+v, err = %v; want tada's record", usage, err)
	}
	for name, body := range map[string]string{
		"a refusal":             `{"ok":false,"error":"not_authed"}`,
		"not JSON":              `<html>`,
		"an emoji_use not JSON": `{"ok":true,"prefs":{"emoji_use":"tada"}}`,
	} {
		if _, err := parseEmojiUsage([]byte(body)); err == nil {
			t.Errorf("%s parsed without an error", name)
		}
	}
}
