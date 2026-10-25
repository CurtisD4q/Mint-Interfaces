// Package mint is a zero-dependency terminal UI library: a rounded purple
// frame, a title and breadcrumb, a home list, a selection bar, value rows,
// sliders, ‹ › pickers, toggles, actions, section headers, a scrollbar and a
// key-hint footer. Keyboard and mouse both work (click selects, double-click
// opens, the wheel scrolls). Redraws are dirty-gated, so an idle screen does
// not repaint.
//
// It is a Go port of Mint Interfaces, the generic interface layer from
// Terminal Settings Manager by Curtis (Apache-2.0), and draws byte-for-byte
// the same frames.
//
// There is no settings logic and no system backend in the library. An app
// describes screens as structs and handles its own data.
package mint
