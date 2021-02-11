package tview

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/pgavlin/markdown-kit/styles"
	"github.com/rivo/tview"
	"github.com/stretchr/testify/require"
)

var testdataPath = filepath.Join("..", "internal", "testdata")

func TestMarkdownView_Basic(t *testing.T) {
	source, err := ioutil.ReadFile(filepath.Join(testdataPath, "getting-started.md"))
	require.NoError(t, err)

	stepsFile, err := ioutil.ReadFile(filepath.Join(testdataPath, "getting-started.view.json"))
	require.NoError(t, err)

	type style struct {
		Foreground tcell.Color    `json:"fg,omitempty"`
		Background tcell.Color    `json:"bg,omitempty"`
		Attributes tcell.AttrMask `json:"attrs,omitempty"`
	}
	type cell struct {
		Bytes string `json:"bytes,omitempty"`
		Style style  `json:"style,omitempty"`
		Runes string `json:"runes,omitempty"`
	}
	type step struct {
		Key   tcell.Key `json:"key,omitempty"`
		Rune  rune      `json:"rune,omitempty"`
		Cells []cell    `json:"cells,omitempty"`
	}
	var steps []step
	err = json.Unmarshal(stepsFile, &steps)
	require.NoError(t, err)

	screen := tcell.NewSimulationScreen("")
	err = screen.Init()
	require.NoError(t, err)
	screen.SetSize(80, 24)

	view := NewMarkdownView(styles.Pulumi)
	view.SetText("getting-started.md", string(source))
	app := tview.NewApplication().SetScreen(screen).SetRoot(view, true)

	redraws := make(chan []tcell.SimCell)
	app.SetAfterDrawFunc(func(_ tcell.Screen) {
		cells, _, _ := screen.GetContents()
		redraws <- append([]tcell.SimCell{}, cells...)
	})

	go app.Run()

	for _, step := range steps {
		if step.Key != 0 {
			app.QueueEvent(tcell.NewEventKey(step.Key, step.Rune, 0))
		}
		simCells := <-redraws

		actual := make([]cell, len(simCells))
		for i, c := range simCells {
			fg, bg, attrs := c.Style.Decompose()
			actual[i] = cell{
				Bytes: string(c.Bytes),
				Style: style{
					Foreground: fg,
					Background: bg,
					Attributes: attrs,
				},
				Runes: string(c.Runes)}
		}

		require.Equal(t, step.Cells, actual)
	}
}
