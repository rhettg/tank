package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// InstanceRow represents a row in the instance table
type InstanceRow struct {
	Name   string
	Status string
	IP     string
}

// RenderInstanceTable renders a styled table of instances
func RenderInstanceTable(rows []InstanceRow) string {
	if len(rows) == 0 {
		return MutedStyle.Render("No instances found")
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().
		Padding(0, 1)

	data := make([][]string, len(rows))
	for i, row := range rows {
		data[i] = []string{row.Name, row.Status, row.IP}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(Muted)).
		Headers("NAME", "STATUS", "IP").
		Rows(data...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	return t.Render()
}

// LayerRow represents a row in the layer table
type LayerRow struct {
	Name    string
	Script  bool
	Files   bool
	Volumes bool
	Hash    string
}

// RenderLayerTable renders a styled table of layers
func RenderLayerTable(base string, rows []LayerRow) string {
	var out string

	out += Bold.Render("Base: ") + Highlight.Render(base) + "\n\n"

	if len(rows) == 0 {
		out += MutedStyle.Render("No layers")
		return out
	}

	out += Bold.Render("Layers:") + "\n"

	for _, row := range rows {
		scriptMark := MutedStyle.Render("·")
		if row.Script {
			scriptMark = SuccessStyle.Render("▪")
		}

		filesMark := MutedStyle.Render("·")
		if row.Files {
			filesMark = SuccessStyle.Render("▪")
		}

		volumesMark := MutedStyle.Render("·")
		if row.Volumes {
			volumesMark = SuccessStyle.Render("▪")
		}

		out += "  " + Bold.Render(row.Name) +
			"  " + scriptMark + " script" +
			"  " + filesMark + " files" +
			"  " + volumesMark + " volumes" +
			"  " + FormatHash(row.Hash) + "\n"
	}

	return out
}

// VolumeRow represents a row in the volume table
type VolumeRow struct {
	Name     string
	Size     string
	Instance string
	Path     string
}

// RenderVolumeTable renders a styled table of volumes
func RenderVolumeTable(rows []VolumeRow) string {
	if len(rows) == 0 {
		return MutedStyle.Render("No volumes found")
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().
		Padding(0, 1)

	data := make([][]string, len(rows))
	for i, row := range rows {
		data[i] = []string{row.Name, row.Size, row.Instance, row.Path}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(Muted)).
		Headers("NAME", "SIZE", "INSTANCE", "PATH").
		Rows(data...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	return t.Render()
}
