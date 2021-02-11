module github.com/pgavlin/markdown-kit

go 1.15

require (
	github.com/alecthomas/chroma v0.8.2
	github.com/atotto/clipboard v0.1.2
	github.com/gdamore/tcell v1.4.0
	github.com/pgavlin/ansicsi v0.0.0-20210128180815-facca45e1fdd
	github.com/pgavlin/goldmark v1.1.33-0.20201111182858-dba88a1da006
	github.com/rivo/tview v0.0.0-00010101000000-000000000000
	github.com/rivo/uniseg v0.2.0
	github.com/skratchdot/open-golang v0.0.0-20200116055534-eef842397966
	github.com/stretchr/testify v1.7.0
	golang.org/x/term v0.0.0-20201210144234-2321bbc49cbf
)

replace github.com/rivo/tview => github.com/pgavlin/tview v0.0.0-20191021225539-6b3fef55089f
