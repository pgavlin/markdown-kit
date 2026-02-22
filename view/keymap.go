package view

import "charm.land/bubbles/v2/key"

// KeyMap defines key bindings for the Model. Each field is a key.Binding that
// can be customized or disabled. Use SetEnabled to bulk-enable/disable all
// bindings for focus management.
type KeyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding

	GotoTop key.Binding
	GotoEnd key.Binding
	Home    key.Binding
	End     key.Binding

	Left  key.Binding
	Right key.Binding

	NextLink      key.Binding
	PrevLink      key.Binding
	NextCodeBlock key.Binding
	PrevCodeBlock key.Binding
	NextHeading   key.Binding
	PrevHeading   key.Binding

	DecreaseWidth key.Binding
	IncreaseWidth key.Binding

	FollowLink key.Binding
	GoBack     key.Binding

	CopySelection key.Binding
}

// DefaultKeyMap returns a KeyMap with the default key bindings matching the
// original hardcoded keys. DecreaseWidth and IncreaseWidth are disabled by
// default since they are unusual for an embedded component.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k/up", "scroll up"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j/down", "scroll down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+b"),
			key.WithHelp("pgup/ctrl+b", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+f"),
			key.WithHelp("pgdown/ctrl+f", "page down"),
		),
		GotoTop: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "go to top"),
		),
		GotoEnd: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "go to end"),
		),
		Home: key.NewBinding(
			key.WithKeys("home"),
			key.WithHelp("home", "go to top"),
		),
		End: key.NewBinding(
			key.WithKeys("end"),
			key.WithHelp("end", "reset column"),
		),
		Left: key.NewBinding(
			key.WithKeys("h", "left"),
			key.WithHelp("h/left", "scroll left"),
		),
		Right: key.NewBinding(
			key.WithKeys("l", "right"),
			key.WithHelp("l/right", "scroll right"),
		),
		NextLink: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next link"),
		),
		PrevLink: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "previous link"),
		),
		NextCodeBlock: key.NewBinding(
			key.WithKeys("ctrl+]"),
			key.WithHelp("ctrl+]", "next code block"),
		),
		PrevCodeBlock: key.NewBinding(
			key.WithKeys("ctrl+["),
			key.WithHelp("ctrl+[", "previous code block"),
		),
		NextHeading: key.NewBinding(
			key.WithKeys("}"),
			key.WithHelp("}", "next heading"),
		),
		PrevHeading: key.NewBinding(
			key.WithKeys("{"),
			key.WithHelp("{", "previous heading"),
		),
		DecreaseWidth: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "decrease width"),
			key.WithDisabled(),
		),
		IncreaseWidth: key.NewBinding(
			key.WithKeys("="),
			key.WithHelp("=", "increase width"),
			key.WithDisabled(),
		),
		FollowLink: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "follow link"),
			key.WithDisabled(),
		),
		GoBack: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("bksp", "go back"),
			key.WithDisabled(),
		),
		CopySelection: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy current selection"),
		),
	}
}

// ShortHelp returns a short list of key bindings for the help view.
// Implements the help.KeyMap interface from charmbracelet/bubbles/help.
func (km KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{km.Up, km.Down, km.NextLink, km.NextHeading, km.FollowLink, km.GoBack}
}

// FullHelp returns the full set of key bindings for the help view.
// Implements the help.KeyMap interface from charmbracelet/bubbles/help.
func (km KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.Up, km.Down, km.PageUp, km.PageDown, km.GotoTop, km.GotoEnd},
		{km.Left, km.Right, km.Home, km.End},
		{km.NextLink, km.PrevLink, km.NextHeading, km.PrevHeading},
		{km.DecreaseWidth, km.IncreaseWidth},
		{km.FollowLink, km.GoBack},
	}
}

// SetEnabled bulk-enables or disables all bindings in the KeyMap.
// This is the primary mechanism for focus management: disable the KeyMap
// when the Model does not have focus, re-enable when it regains focus.
func (km *KeyMap) SetEnabled(enabled bool) {
	bindings := []*key.Binding{
		&km.Up, &km.Down, &km.PageUp, &km.PageDown,
		&km.GotoTop, &km.GotoEnd, &km.Home, &km.End,
		&km.Left, &km.Right,
		&km.NextLink, &km.PrevLink,
		&km.NextHeading, &km.PrevHeading,
		&km.DecreaseWidth, &km.IncreaseWidth,
		&km.FollowLink, &km.GoBack,
	}
	for _, b := range bindings {
		b.SetEnabled(enabled)
	}
}
