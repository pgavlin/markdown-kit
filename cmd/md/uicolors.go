package main

import "charm.land/lipgloss/v2"

// UI color palette — central definitions for the terminal UI chrome.
// Content/syntax colors come from the styles.Theme; these are for
// interactive elements (pickers, tabs, overlays, etc.).
var (
	colorAccent      = lipgloss.Color("212") // pink — cursor, selected item text
	colorMuted       = lipgloss.Color("240") // gray — secondary text, hints, paths, borders
	colorDirectory   = lipgloss.Color("99")  // purple — directory names in file picker
	colorDisabled    = lipgloss.Color("243") // dim gray — disabled items
	colorDisabledAlt = lipgloss.Color("247") // lighter gray — disabled cursor/selected
	colorPermission  = lipgloss.Color("244") // mid gray — file permissions
	colorTabActiveFg = lipgloss.Color("255") // white — active tab foreground
	colorTabActiveBg = lipgloss.Color("62")  // steel blue — active tab background
)
