package ui

import (
	"strings"

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
	Cache   LayerCacheStatus
}

// LayerCacheStatus represents the cache state for a layer.
type LayerCacheStatus string

const (
	LayerCacheCached LayerCacheStatus = "cached"
	LayerCacheBuild  LayerCacheStatus = "build"
	LayerCacheNA     LayerCacheStatus = "n/a"
)

// RenderLayerTable renders a styled table of layers
func RenderLayerTable(base string, baseCached bool, rows []LayerRow) string {
	var out strings.Builder

	baseStatus := MutedStyle.Render("(not cached)")
	if baseCached {
		baseStatus = SuccessStyle.Render("(cached)")
	}

	out.WriteString(Bold.Render("Base: ") + Highlight.Render(base) + " " + baseStatus + "\n\n")

	if len(rows) == 0 {
		out.WriteString(MutedStyle.Render("No layers"))
		return out.String()
	}

	data := make([][]string, len(rows))
	for i, row := range rows {
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

		cacheLabel := renderLayerCache(row.Cache)

		data[i] = []string{
			row.Name,
			scriptMark,
			filesMark,
			volumesMark,
			cacheLabel,
			FormatHash(row.Hash),
		}
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(Muted)).
		Headers("NAME", "SCRIPT", "FILES", "VOLUMES", "CACHE", "HASH").
		Rows(data...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	out.WriteString(t.Render())
	return out.String()
}

func renderLayerCache(status LayerCacheStatus) string {
	switch status {
	case LayerCacheCached:
		return SuccessStyle.Render(string(status))
	case LayerCacheBuild:
		return Highlight.Render(string(status))
	case LayerCacheNA:
		return MutedStyle.Render(string(status))
	default:
		return MutedStyle.Render("unknown")
	}
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
