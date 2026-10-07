// Copyright (C) 2026 Yota Hamada
// SPDX-License-Identifier: GPL-3.0-or-later

package browser

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureSnapshot reads a page's accessibility tree, as Chrome reported it,
// and its link addresses from testdata/outline.
func fixtureSnapshot(t *testing.T, name string) pageSnapshot {
	t.Helper()
	tree, err := os.ReadFile(filepath.Join("testdata", "outline", name+".tree"))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join("testdata", "outline", name+".urls.json"))
	require.NoError(t, err)
	var urls map[string]string
	require.NoError(t, json.Unmarshal(data, &urls))
	return pageSnapshot{Tree: string(tree), URLs: urls}
}

// The outline shows what a person decides from: headings, fields, buttons,
// links with their addresses, messages, and tables and lists with repeated
// rows collapsed, nested in the landmarks and frames that hold them.
func TestOutlineDescribesWhatAPersonSees(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ fixture, want string }{
		{"login", `banner
  navigation
    link "Home" -> https://portal.example.com/
    link "Help" -> https://portal.example.com/help
heading: 取引先ポータル
form
  textbox "ログインID"
  textbox "パスワード"
  checkbox "Remember me" [checked]
  button "ログイン"
alert: IDまたはパスワードが違います。`},
		{"orders", `navigation
  link "注文一覧" -> https://portal.example.com/orders
  link "請求書" -> https://portal.example.com/invoices
heading: 注文一覧
select "状態" = 未出荷; options: すべて, 未出荷, 出荷済み
button "検索"
table: 12 rows; columns: 注文番号 | 取引先 | 金額 | 状態
  row: PO-01 | Acme | 12,000円 | 未出荷
    link "PO-01" -> https://portal.example.com/orders/PO-01
  row: PO-02 | Acme | 12,000円 | 未出荷
    link "PO-02" -> https://portal.example.com/orders/PO-02
  row: PO-03 | Acme | 12,000円 | 未出荷
    link "PO-03" -> https://portal.example.com/orders/PO-03
  … 9 more rows like these
button "前へ"
text: 12件中 1 / 3 ページ
button "次へ"`},
		{"list", `heading: Items
list
  item: Item B ¥100
    link "Item B" -> https://portal.example.com/item/b
  item: Item C ¥200
    link "Item C" -> https://portal.example.com/item/c
  item: Item D ¥300
    link "Item D" -> https://portal.example.com/item/d
  … 27 more items like these
radio "Newest" [checked]
radio "Price"
textbox "Note"`},
		{"frame", `heading: Outer
iframe "login frame"
  banner
    navigation
      link "Home" -> https://portal.example.com/
      link "Help" -> https://portal.example.com/help
  heading: 取引先ポータル
  form
    textbox "ログインID"
    textbox "パスワード"
    checkbox "Remember me" [checked]
    button "ログイン"
  alert: IDまたはパスワードが違います。`},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			t.Parallel()
			text, truncated, _ := renderOutline(fixtureSnapshot(t, tc.fixture), outlineOptions{MaxChars: 8000})
			assert.False(t, truncated)
			assert.Equal(t, tc.want, text)
		})
	}
}

// A search shows every entry containing the text, whatever its case and
// however deep in a collapsed run, with the entries it sits in.
func TestOutlineFind(t *testing.T) {
	t.Parallel()

	snap := fixtureSnapshot(t, "orders")
	text, _, matches := renderOutline(snap, outlineOptions{Find: "po-07"})
	assert.Equal(t, 1, matches)
	assert.Equal(t, `table: 12 rows; columns: 注文番号 | 取引先 | 金額 | 状態
  row: PO-07 | Acme | 12,000円 | 未出荷
    link "PO-07" -> https://portal.example.com/orders/PO-07`, text)

	text, _, matches = renderOutline(snap, outlineOptions{Find: "orders/PO-1"})
	assert.Equal(t, 3, matches, "the address of a link is searched too")
	assert.Equal(t, 1, strings.Count(text, "table:"), "matches in one table share its line")

	text, _, matches = renderOutline(snap, outlineOptions{Find: "invoice #42"})
	assert.Zero(t, matches)
	assert.Empty(t, text)
}

// An outline over its limit ends at a whole line and says how to see more.
func TestOutlineFitsItsLimit(t *testing.T) {
	t.Parallel()

	text, truncated, _ := renderOutline(fixtureSnapshot(t, "orders"), outlineOptions{MaxChars: 200})
	assert.True(t, truncated)
	assert.LessOrEqual(t, len(text), 200+len("… outline cut: 99 more lines; narrow it with find or allow more characters"))
	lines := strings.Split(text, "\n")
	assert.Equal(t, `heading: 注文一覧`, lines[3])
	assert.Regexp(t, `^… outline cut: \d+ more lines; narrow it with find or allow more characters$`, lines[len(lines)-1])
}

// What was typed into a field never shows, whether the field holds it as
// text or names it as its value.
func TestOutlineLeavesOutTypedText(t *testing.T) {
	t.Parallel()

	tree := `[0-1] RootWebArea: Sign in
  [0-2] textbox: Password
    [0-3] StaticText: s3cret-value
  [0-4] combobox: Search
    [0-5] StaticText: typed query
  [0-6] paragraph
    [0-7] StaticText: Welcome back`
	text, _, _ := renderOutline(pageSnapshot{Tree: tree}, outlineOptions{})
	assert.Equal(t, `textbox "Password"
combobox "Search"
text: Welcome back`, text)
}

// A select lists its first options and counts the rest, and a link that
// runs script shows no address.
func TestOutlineChoicesAndScriptLinks(t *testing.T) {
	t.Parallel()

	var tree strings.Builder
	tree.WriteString("[0-1] RootWebArea: Settings\n  [0-2] select: Country\n")
	for i := range 20 {
		flag := ""
		if i == 4 {
			flag = " [selected]"
		}
		fmt.Fprintf(&tree, "    [0-%d] option: Country %d%s\n", 10+i, i, flag)
	}
	tree.WriteString("  [0-3] link: Open menu\n  [0-4] link: Name: with colon\n")
	text, _, _ := renderOutline(pageSnapshot{Tree: tree.String(), URLs: map[string]string{
		"0-3": "javascript:void(0)",
		"0-4": "https://example.com/a",
	}}, outlineOptions{})
	assert.Equal(t, `select "Country" = Country 4; options: Country 0, Country 1, Country 2, Country 3, Country 4, Country 5, Country 6, Country 7, Country 8, Country 9, Country 10, Country 11, Country 12, Country 13, Country 14 (+5 more)
link "Open menu"
link "Name: with colon" -> https://example.com/a`, text)
}

// A tree whose lines end in CRLF, as a fixture checked out on Windows has,
// reads as the same tree.
func TestOutlineReadsCRLFTrees(t *testing.T) {
	t.Parallel()

	snap := fixtureSnapshot(t, "login")
	want, _, _ := renderOutline(snap, outlineOptions{})
	snap.Tree = strings.ReplaceAll(strings.ReplaceAll(snap.Tree, "\r\n", "\n"), "\n", "\r\n")
	got, _, _ := renderOutline(snap, outlineOptions{})
	assert.Equal(t, want, got)
}
