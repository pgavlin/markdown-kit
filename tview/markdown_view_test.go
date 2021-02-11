package tview

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/gdamore/tcell"
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

	type cell struct {
		Bytes string      `json:"bytes,omitempty"`
		Style tcell.Style `json:"style,omitempty"`
		Runes string      `json:"runes,omitempty"`
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
		for i, c := range step.Cells {
			actual[i] = cell{Bytes: string(c.Bytes), Style: c.Style, Runes: string(c.Runes)}
		}

		require.Equal(t, step.Cells, actual)
	}
}
